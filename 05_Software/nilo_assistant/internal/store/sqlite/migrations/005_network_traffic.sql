-- Twenty-four hours of one-minute interface traffic aggregates.
--
-- A minute the sampler could not measure is simply absent: there is no row for
-- it, and no reader may invent one. That is what keeps "no traffic" and "no
-- measurement" distinguishable, which a zero-filled series would destroy.
--
-- samples records how many five-second intervals composed the minute, so a row
-- assembled from four readings can be told from one assembled from eleven.
-- measured_by records what observed it, so a simulated figure can never be read
-- as a bench observation.

CREATE TABLE IF NOT EXISTS network_traffic_minute (
    interface_id TEXT    NOT NULL,
    -- Unix seconds, UTC, always minute-aligned. Stored as an integer so the
    -- retention sweep and the period window are plain range comparisons.
    minute_start INTEGER NOT NULL,

    rx_bytes   INTEGER NOT NULL,
    tx_bytes   INTEGER NOT NULL,
    rx_errors  INTEGER NOT NULL,
    tx_errors  INTEGER NOT NULL,
    rx_dropped INTEGER NOT NULL,
    tx_dropped INTEGER NOT NULL,

    samples     INTEGER NOT NULL CHECK (samples > 0),
    measured_by TEXT    NOT NULL CHECK (measured_by <> ''),

    PRIMARY KEY (interface_id, minute_start)
) STRICT;

-- The retention sweep deletes by age across every interface at once.
CREATE INDEX IF NOT EXISTS network_traffic_minute_by_age
    ON network_traffic_minute (minute_start);
