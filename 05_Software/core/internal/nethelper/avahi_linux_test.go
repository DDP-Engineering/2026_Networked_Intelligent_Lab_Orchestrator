//go:build linux

package nethelper

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestAvahiMissingGroupErrorsAreAlreadyWithdrawn(t *testing.T) {
	for _, name := range []string{
		"org.freedesktop.DBus.Error.UnknownObject",
		"org.freedesktop.DBus.Error.ServiceUnknown",
		"org.freedesktop.DBus.Error.NameHasNoOwner",
		"org.freedesktop.Avahi.InvalidObject",
	} {
		if !avahiGroupMissing(dbus.Error{Name: name}) {
			t.Fatalf("%s was not recognized as a vanished owned group", name)
		}
	}
	if avahiGroupMissing(dbus.Error{Name: "org.freedesktop.DBus.Error.AccessDenied"}) {
		t.Fatal("an unrelated Avahi failure was treated as already withdrawn")
	}
}
