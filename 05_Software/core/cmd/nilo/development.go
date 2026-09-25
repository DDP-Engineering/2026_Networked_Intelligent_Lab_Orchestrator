package main

import (
	"context"
	"errors"
	"fmt"

	"nilo.local/nilo/internal/app"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
)

// The development administrator's fixed credentials.
//
// They are deliberately the same pair the browser mock seeds, so a developer
// learns one username and one password and it works in both places.
const (
	DevelopmentAdministratorUsername = "admin"
	DevelopmentAdministratorPassword = "admin_password"

	// simulationProfileName is the only profile this convenience exists on.
	simulationProfileName = "simulation"
)

// errDevelopmentAdministratorProfile refuses the fixed password anywhere but
// the simulation profile.
var errDevelopmentAdministratorProfile = errors.New(
	"the development administrator is only available on the simulation profile")

// ensureDevelopmentAdministrator creates a first administrator with a fixed,
// published password so a developer can start the appliance and sign in with no
// bootstrap step and no token to issue.
//
// It is a development convenience and is fenced as one. It refuses on any
// profile but simulation, and it only ever creates the *first* administrator: an
// appliance that already has one keeps it, with its own password, untouched. So
// the flag cannot reset a real credential, and it cannot exist on a real
// appliance at all.
//
// It reports whether it created the administrator, so the caller can say what
// actually happened rather than printing a credential that may not be in use.
func ensureDevelopmentAdministrator(
	ctx context.Context, application *app.App, profile string,
) (bool, error) {
	if profile != simulationProfileName {
		return false, fmt.Errorf("%w: profile is %q", errDevelopmentAdministratorProfile, profile)
	}

	service := application.IdentityService()

	_, err := service.Bootstrap(
		ctx, DevelopmentAdministratorUsername, []byte(DevelopmentAdministratorPassword))
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, identity.ErrAdministratorExists) {
		return false, fmt.Errorf("create the development administrator: %w", err)
	}

	// An administrator already exists. If it is this same development identity —
	// left over from an earlier run of this data directory — its password is
	// reset to the fixed one, because a development credential that silently
	// stops working is worse than no credential at all. Any *other*
	// administrator is somebody's real account and is never touched.
	users, err := service.ListUsers(ctx)
	if err != nil {
		return false, fmt.Errorf("read the existing identities: %w", err)
	}
	for _, user := range users {
		if user.Username != DevelopmentAdministratorUsername || user.Role != auth.RoleAdministrator {
			continue
		}
		if err := service.ResetPassword(
			ctx, user, DevelopmentAdministratorUsername,
			[]byte(DevelopmentAdministratorPassword)); err != nil {
			return false, fmt.Errorf("reset the development administrator password: %w", err)
		}
		return true, nil
	}
	return false, nil
}

// developmentAdministratorNotice is what the server prints about the fixed
// credential. A password nobody can see is worse than useless, and a notice
// that claims a creation which did not happen is worse still.
func developmentAdministratorNotice(created bool) string {
	if !created {
		return fmt.Sprintf(
			"development administrator: an administrator already exists on this data directory, "+
				"so %q was not added and no password was changed",
			DevelopmentAdministratorUsername)
	}
	return fmt.Sprintf(
		"development administrator created: sign in as %q with the password %q. "+
			"This is a fixed development credential on the simulation profile; it is refused on every other profile.",
		DevelopmentAdministratorUsername, DevelopmentAdministratorPassword)
}
