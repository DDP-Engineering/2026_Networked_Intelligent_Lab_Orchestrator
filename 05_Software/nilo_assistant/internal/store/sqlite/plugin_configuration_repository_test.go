package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestPluginConfigurationUsesRevisionCheckedWrites(t *testing.T) {
	repository := NewPluginConfigurationRepository(openTemporaryDatabase(t))
	ctx := context.Background()
	initial, err := repository.Get(ctx, "camera")
	if err != nil || initial.Revision != 0 || string(initial.Settings) != `{}` {
		t.Fatalf("initial configuration = %+v, %v", initial, err)
	}
	updated, err := repository.Put(ctx, "camera", 0, json.RawMessage(`{"maximum_width":1280}`), time.Now())
	if err != nil || updated.Revision != 1 {
		t.Fatalf("updated configuration = %+v, %v", updated, err)
	}
	if _, err := repository.Put(ctx, "camera", 0, json.RawMessage(`{}`), time.Now()); !errors.Is(err, ErrManagedDeviceConflict) {
		t.Fatalf("stale configuration write = %v", err)
	}
}
