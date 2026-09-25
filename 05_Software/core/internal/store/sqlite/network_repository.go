package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"nilo.local/nilo/internal/network"
)

// NetworkRepository stores the rolling twenty-four hours of one-minute
// interface traffic aggregates.
//
// It stores only minutes that were measured. There is no fill, no interpolation
// and no zero row for a minute nothing was observed in, because a reader has to
// be able to tell an idle interface from an unread one.
type NetworkRepository struct {
	database *DB
}

func NewNetworkRepository(database *DB) *NetworkRepository {
	return &NetworkRepository{database: database}
}

func (repository *NetworkRepository) ready() error {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return errors.New("network repository database is required")
	}
	return nil
}

// WriteAggregate stores one closed minute.
//
// A minute is closed once, by one sampler, so a repeat is a retry rather than
// new traffic: the first stored value wins and the repeat changes nothing. That
// keeps a restarted or duplicated sampler from doubling a figure.
func (repository *NetworkRepository) WriteAggregate(ctx context.Context, aggregate network.Aggregate) error {
	if err := repository.ready(); err != nil {
		return err
	}
	if aggregate.InterfaceID == "" {
		return errors.New("traffic aggregate requires an interface identity")
	}
	if aggregate.Samples <= 0 {
		return errors.New("traffic aggregate requires at least one contributing sample")
	}
	if aggregate.MeasuredBy == "" {
		return errors.New("traffic aggregate requires the name of what measured it")
	}
	minuteStart := aggregate.MinuteStart.UTC().Truncate(time.Minute)

	if _, err := repository.database.SQL().ExecContext(ctx, `
		INSERT INTO network_traffic_minute (
			interface_id, minute_start,
			rx_bytes, tx_bytes, rx_errors, tx_errors, rx_dropped, tx_dropped,
			samples, measured_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (interface_id, minute_start) DO NOTHING
	`,
		aggregate.InterfaceID, minuteStart.Unix(),
		int64(aggregate.RXBytes), int64(aggregate.TXBytes),
		int64(aggregate.RXErrors), int64(aggregate.TXErrors),
		int64(aggregate.RXDropped), int64(aggregate.TXDropped),
		int64(aggregate.Samples), string(aggregate.MeasuredBy),
	); err != nil {
		return fmt.Errorf("store traffic aggregate: %w", err)
	}
	return nil
}

// PruneBefore drops every aggregate older than the cutoff, across all
// interfaces. It is how the twenty-four-hour window stays rolling.
func (repository *NetworkRepository) PruneBefore(ctx context.Context, cutoff time.Time) error {
	if err := repository.ready(); err != nil {
		return err
	}
	if _, err := repository.database.SQL().ExecContext(ctx,
		"DELETE FROM network_traffic_minute WHERE minute_start < ?",
		cutoff.UTC().Unix(),
	); err != nil {
		return fmt.Errorf("prune expired traffic aggregates: %w", err)
	}
	return nil
}

// History returns the stored minutes of one interface within [from, to), in
// ascending order. The half-open window is what makes consecutive periods
// tile without repeating their shared boundary minute.
func (repository *NetworkRepository) History(
	ctx context.Context, interfaceID string, from, to time.Time,
) (aggregates []network.Aggregate, err error) {
	if err := repository.ready(); err != nil {
		return nil, err
	}
	if interfaceID == "" {
		return nil, errors.New("traffic history requires an interface identity")
	}
	if !to.After(from) {
		return nil, errors.New("traffic history requires a period that ends after it starts")
	}

	rows, err := repository.database.SQL().QueryContext(ctx, `
		SELECT minute_start,
		       rx_bytes, tx_bytes, rx_errors, tx_errors, rx_dropped, tx_dropped,
		       samples, measured_by
		FROM network_traffic_minute
		WHERE interface_id = ? AND minute_start >= ? AND minute_start < ?
		ORDER BY minute_start
	`, interfaceID, from.UTC().Unix(), to.UTC().Unix())
	if err != nil {
		return nil, fmt.Errorf("read traffic history: %w", err)
	}
	defer func() {
		closeErr := rows.Close()
		if err == nil && closeErr != nil {
			err = fmt.Errorf("close traffic history rows: %w", closeErr)
		}
	}()

	for rows.Next() {
		var minuteStart, rxBytes, txBytes, rxErrors, txErrors, rxDropped, txDropped, samples int64
		var measuredBy string
		if err := rows.Scan(
			&minuteStart, &rxBytes, &txBytes, &rxErrors, &txErrors,
			&rxDropped, &txDropped, &samples, &measuredBy,
		); err != nil {
			return nil, fmt.Errorf("scan traffic history row: %w", err)
		}
		aggregates = append(aggregates, network.Aggregate{
			InterfaceID: interfaceID,
			MinuteStart: time.Unix(minuteStart, 0).UTC(),
			RXBytes:     uint64(rxBytes), TXBytes: uint64(txBytes),
			RXErrors: uint64(rxErrors), TXErrors: uint64(txErrors),
			RXDropped: uint64(rxDropped), TXDropped: uint64(txDropped),
			Samples: int(samples), MeasuredBy: network.Source(measuredBy),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate traffic history: %w", err)
	}
	return aggregates, nil
}
