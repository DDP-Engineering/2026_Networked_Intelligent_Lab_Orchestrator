package appliance

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type namingRepository struct {
	identity Identity
	err      error
	events   *[]string
}

func (repository *namingRepository) SetFriendlyName(_ context.Context, value string) (Identity, error) {
	*repository.events = append(*repository.events, "persist:"+value)
	if repository.err != nil {
		return Identity{}, repository.err
	}
	repository.identity.FriendlyName = value
	return repository.identity, nil
}

type namingCertificates struct {
	events *[]string
}

func (certificates namingCertificates) ActivateNames(_ context.Context, values []string) error {
	*certificates.events = append(*certificates.events, "certificate:"+strings.Join(values, ","))
	return nil
}

type namingPublisher struct {
	events *[]string
	err    error
}

type faultingNamingCertificates struct {
	events *[]string
	calls  int
	failAt int
}

func (certificates *faultingNamingCertificates) ActivateNames(_ context.Context, values []string) error {
	certificates.calls++
	*certificates.events = append(*certificates.events, "certificate:"+strings.Join(values, ","))
	if certificates.calls == certificates.failAt {
		return errors.New("certificate failure")
	}
	return nil
}

func (publisher namingPublisher) ReplaceFriendlyName(_ context.Context, value string) error {
	*publisher.events = append(*publisher.events, "publish:"+value)
	return publisher.err
}

func TestNamingServiceCoversThenPublishesThenPersists(t *testing.T) {
	events := []string{}
	repository := &namingRepository{events: &events, identity: Identity{
		ID: "0199a2d0-1234-7abc-8def-0123456789ab", CommissionedAt: time.Now().UTC(),
	}}
	service, err := NewNamingService("nilo-p2rjmzdkf1", "", repository, namingCertificates{&events}, namingPublisher{events: &events})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := service.SetFriendlyName(context.Background(), "bench-west")
	if err != nil || identity.FriendlyName != "bench-west" {
		t.Fatalf("SetFriendlyName = %+v, %v", identity, err)
	}
	want := []string{
		"certificate:nilo-p2rjmzdkf1,nilo,bench-west",
		"publish:bench-west",
		"persist:bench-west",
		"certificate:nilo-p2rjmzdkf1,nilo,bench-west",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestNamingServiceRollsBackPublicationAndCoverageOnPersistenceFailure(t *testing.T) {
	events := []string{}
	repository := &namingRepository{events: &events, err: errors.New("disk full")}
	service, err := NewNamingService("nilo-p2rjmzdkf1", "bench-old", repository, namingCertificates{&events}, namingPublisher{events: &events})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetFriendlyName(context.Background(), "bench-west"); err == nil {
		t.Fatal("persistence failure was accepted")
	}
	wantTail := []string{"publish:bench-old", "certificate:nilo-p2rjmzdkf1,nilo,bench-old"}
	if len(events) < 2 || !reflect.DeepEqual(events[len(events)-2:], wantTail) {
		t.Fatalf("rollback events = %v, want suffix %v", events, wantTail)
	}
}

func TestNamingServiceRollsBackDurableChangeWhenFinalCertificateCleanupFails(t *testing.T) {
	events := []string{}
	repository := &namingRepository{events: &events, identity: Identity{
		ID: "0199a2d0-1234-7abc-8def-0123456789ab", FriendlyName: "bench-old", CommissionedAt: time.Now().UTC(),
	}}
	certificates := &faultingNamingCertificates{events: &events, failAt: 2}
	service, err := NewNamingService("nilo-p2rjmzdkf1", "bench-old", repository, certificates, namingPublisher{events: &events})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetFriendlyName(context.Background(), "bench-west"); err == nil {
		t.Fatal("final certificate failure was reported as success")
	}
	if repository.identity.FriendlyName != "bench-old" {
		t.Fatalf("durable friendly name = %q", repository.identity.FriendlyName)
	}
	wantTail := []string{"publish:bench-old", "persist:bench-old", "certificate:nilo-p2rjmzdkf1,nilo,bench-old"}
	if len(events) < len(wantTail) || !reflect.DeepEqual(events[len(events)-len(wantTail):], wantTail) {
		t.Fatalf("rollback events = %v, want suffix %v", events, wantTail)
	}
}
