package nethelper

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func stagingEngine(t *testing.T) (*Engine, *Keystore) {
	t.Helper()
	keystore, _ := newKeystore(t)
	engine, err := NewEngine(&fakeDriver{}, &memoryStore{}, func() time.Time {
		return time.Date(2026, 9, 3, 20, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}
	if err := engine.ConfigureKeystore(keystore); err != nil {
		t.Fatalf("configure keystore: %v", err)
	}
	return engine, keystore
}

// TestStagingOverTheProtocolBindsKeyMaterialToAProfile is the round trip the
// unprivileged core actually makes: hand the key over once, get a handle, bind
// it after the import succeeds.
func TestStagingOverTheProtocolBindsKeyMaterialToAProfile(t *testing.T) {
	ctx := context.Background()
	engine, keystore := stagingEngine(t)

	staged := dispatch(ctx, engine, Request{
		Op: "wireguard.secrets.stage",
		Secret: &SecretPayload{
			PrivateKey:    sentinelPrivate,
			PresharedKeys: []string{sentinelPreshared, ""},
		},
	})
	if staged.Error != "" {
		t.Fatalf("stage: %s", staged.Error)
	}
	if staged.Handle == "" {
		t.Fatal("staging returned no handle")
	}

	// The response is what crosses back. It may carry the handle and nothing else.
	encoded, err := json.Marshal(staged)
	if err != nil {
		t.Fatalf("encode response: %v", err)
	}
	for _, sentinel := range []string{sentinelPrivate, sentinelPreshared, "private_key"} {
		if strings.Contains(string(encoded), sentinel) {
			t.Fatalf("the staging response carries %q: %s", sentinel, encoded)
		}
	}

	promoted := dispatch(ctx, engine, Request{
		Op: "wireguard.secrets.promote", Handle: staged.Handle, ProfileID: "profile-office",
	})
	if promoted.Error != "" {
		t.Fatalf("promote: %s", promoted.Error)
	}
	stored, err := keystore.Read("profile-office")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if stored.PrivateKey != sentinelPrivate {
		t.Fatal("the promoted key is not the staged one")
	}
	if len(stored.PresharedKeys) != 2 || stored.PresharedKeys[1] != "" {
		t.Fatalf("peer alignment was lost across the socket: %d entries", len(stored.PresharedKeys))
	}

	// Discarding is the other terminal state, and it is the one an abandoned
	// import takes.
	second := dispatch(ctx, engine, Request{
		Op: "wireguard.secrets.stage", Secret: &SecretPayload{PrivateKey: sentinelPrivate},
	})
	if second.Error != "" {
		t.Fatalf("stage second: %s", second.Error)
	}
	if dropped := dispatch(ctx, engine, Request{
		Op: "wireguard.secrets.discard", Handle: second.Handle,
	}); dropped.Error != "" {
		t.Fatalf("discard: %s", dropped.Error)
	}
	if again := dispatch(ctx, engine, Request{
		Op: "wireguard.secrets.promote", Handle: second.Handle, ProfileID: "profile-late",
	}); again.Error == "" {
		t.Fatal("a discarded handle was still promotable")
	}
}

// TestOnlyStagingMayCarryKeyMaterial is the guard that keeps the secret hop to
// exactly one operation. Without it, key material could ride along on any
// request and reach code that has no reason to see it.
func TestOnlyStagingMayCarryKeyMaterial(t *testing.T) {
	ctx := context.Background()
	engine, _ := stagingEngine(t)

	for _, operation := range []string{
		"interface.list", "wireguard.check", "recovery.read",
		"wireguard.secrets.promote", "wireguard.secrets.discard",
	} {
		response := dispatch(ctx, engine, Request{
			Op: operation, Secret: &SecretPayload{PrivateKey: sentinelPrivate},
		})
		if response.Code != "invalid_request" {
			t.Fatalf("%s accepted key material: code=%q error=%q", operation, response.Code, response.Error)
		}
	}

	// A handle is not a secret, but it still has no business on an unrelated
	// operation: accepting it there would mean the guard is decorative.
	for _, operation := range []string{"interface.list", "change.apply", "recovery.reconcile"} {
		response := dispatch(ctx, engine, Request{Op: operation, Handle: "abc", ProfileID: "profile-x"})
		if response.Code != "invalid_request" {
			t.Fatalf("%s accepted a key-material reference: code=%q", operation, response.Code)
		}
	}

	if refused := dispatch(ctx, engine, Request{Op: "wireguard.secrets.stage"}); refused.Code != "invalid_request" {
		t.Fatalf("staging without key material was accepted: %+v", refused)
	}
	if refused := dispatch(ctx, engine, Request{
		Op: "wireguard.secrets.stage", ID: "op-1", Secret: &SecretPayload{PrivateKey: sentinelPrivate},
	}); refused.Code != "invalid_request" {
		t.Fatal("staging accepted an operation identity it has no use for")
	}
}

// TestAHelperWithoutKeyStorageRefusesCustody keeps a misconfigured helper from
// accepting key material and dropping it, which would look like success.
func TestAHelperWithoutKeyStorageRefusesCustody(t *testing.T) {
	engine, err := NewEngine(&fakeDriver{}, &memoryStore{}, time.Now)
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}
	response := dispatch(context.Background(), engine, Request{
		Op: "wireguard.secrets.stage", Secret: &SecretPayload{PrivateKey: sentinelPrivate},
	})
	if response.Error == "" {
		t.Fatal("a helper with no key storage accepted key material")
	}
	if response.Handle != "" {
		t.Fatalf("a refused staging returned a handle %q", response.Handle)
	}
}

// TestTheResponseTypeHasNowhereToPutASecret is structural rather than
// behavioural: it fails if anybody later adds a field to Response that could
// carry key material, which is the mistake a review is most likely to miss.
func TestTheResponseTypeHasNowhereToPutASecret(t *testing.T) {
	payload := reflect.TypeOf(SecretPayload{})
	var walk func(reflect.Type, string, int)
	walk = func(candidate reflect.Type, path string, depth int) {
		if depth > 6 {
			return
		}
		for candidate.Kind() == reflect.Pointer || candidate.Kind() == reflect.Slice {
			candidate = candidate.Elem()
		}
		if candidate == payload {
			t.Fatalf("Response can carry key material at %s", path)
		}
		if candidate.Kind() != reflect.Struct {
			return
		}
		for index := range candidate.NumField() {
			field := candidate.Field(index)
			name := strings.ToLower(field.Name)
			named := strings.Contains(name, "privatekey") || strings.Contains(name, "presharedkey") ||
				strings.Contains(name, "password") || strings.Contains(name, "secret")
			// A handle or a reference is an identity, not a value. That is the
			// whole premise of the staging design, and it is asserted directly
			// elsewhere: the handle a stage returns does not contain the key.
			if strings.Contains(name, "handle") || strings.Contains(name, "ref") {
				named = false
			}
			// A bool named HasPresharedKey is the safe design, not a leak: it
			// exists precisely so the key does not. Only a field that could hold
			// the value itself counts.
			if named && carriesText(field.Type) && field.Type.Name() != "Secrets" {
				t.Fatalf("Response reaches a key-material field at %s.%s (%s)", path, field.Name, field.Type)
			}
			walk(field.Type, path+"."+field.Name, depth+1)
		}
	}
	walk(reflect.TypeOf(Response{}), "Response", 0)

	// The same walk over Request must find the payload, or the test is passing
	// because it cannot see anything rather than because there is nothing there.
	var reachable bool
	var find func(reflect.Type, int)
	find = func(candidate reflect.Type, depth int) {
		if depth > 6 || reachable {
			return
		}
		for candidate.Kind() == reflect.Pointer || candidate.Kind() == reflect.Slice {
			candidate = candidate.Elem()
		}
		if candidate == payload {
			reachable = true
			return
		}
		if candidate.Kind() != reflect.Struct {
			return
		}
		for index := range candidate.NumField() {
			find(candidate.Field(index).Type, depth+1)
		}
	}
	find(reflect.TypeOf(Request{}), 0)
	if !reachable {
		t.Fatal("the walk cannot reach SecretPayload even on Request, so it proves nothing about Response")
	}
}

// carriesText reports whether a type could hold a key as a value.
func carriesText(candidate reflect.Type) bool {
	for candidate.Kind() == reflect.Pointer || candidate.Kind() == reflect.Slice {
		candidate = candidate.Elem()
	}
	return candidate.Kind() == reflect.String || candidate.Kind() == reflect.Struct
}
