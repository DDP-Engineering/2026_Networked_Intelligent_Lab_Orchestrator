package network

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestProfilePreviewBindsParametersAndReturnsNoSecrets(t *testing.T) {
	s := NewSimulation(time.Now)
	m, _ := NewOperationManager(OperationManagerOptions{Changer: s, Observer: s, Now: time.Now, NewID: sequentialIDs()})
	c := Change{Action: ActionProfileImport, Target: "office", ProfileText: fullTunnel}
	p, err := m.PreviewChange(context.Background(), firstActor, c)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(p)
	if strings.Contains(string(encoded), "PrivateKey") || strings.Contains(string(encoded), "aGVsbG8") {
		t.Fatal("preview leaked profile")
	}
	if _, err = m.Acknowledge(context.Background(), firstActor, p.ID, p.Revision); err != nil {
		t.Fatal(err)
	}
	status, _ := s.WireGuard(context.Background())
	if len(status.Profiles) != 1 {
		t.Fatal("acknowledged import did not create profile")
	}
	for _, enabled := range []bool{true, false} {
		q, err := m.PreviewChange(context.Background(), firstActor, Change{Action: ActionAutostartSelect, Target: status.Profiles[0].ID, Enabled: enabled})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = m.Acknowledge(context.Background(), firstActor, q.ID, q.Revision); err != nil {
			t.Fatal(err)
		}
		status, _ = s.WireGuard(context.Background())
		if status.Profiles[0].AutoConnect != enabled {
			t.Fatal("enabled parameter was lost")
		}
	}
}
