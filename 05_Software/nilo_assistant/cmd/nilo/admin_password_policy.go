package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
)

// `nilo admin password-policy` reads and replaces the appliance's one password
// policy. It is a remote-mode command only: the policy belongs to the
// appliance, and the appliance authorizes the token and applies the rule on
// every request. Nothing here decides what a policy may be.

const passwordPolicyPath = "/api/v1/password-policy"

func runAdminPasswordPolicy(ctx context.Context, arguments []string, streams adminIO) int {
	flags := flag.NewFlagSet("admin password-policy", flag.ContinueOnError)
	flags.SetOutput(streams.stderr)
	var options adminOptions
	options.bind(flags)
	minimumLength := flags.Int("minimum-length", 0, "fewest bytes a password may be")
	minimumClasses := flags.Int("minimum-classes", -1,
		"fewest distinct character classes a password must carry, from lowercase, uppercase, digit and other")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 {
		fmt.Fprintln(streams.stderr, adminUsage)
		return 2
	}
	mode, err := options.mode("")
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		fmt.Fprintln(streams.stderr, adminUsage)
		return 2
	}
	if mode != adminModeRemote {
		fmt.Fprintln(streams.stderr,
			"password-policy is a remote-mode command: pass --server and --token-file instead of --data-dir")
		return 2
	}

	// Half a policy is refused here rather than sent, for the reason the route
	// refuses one: a merged change would silently relax the half not given.
	lengthGiven, classesGiven := *minimumLength != 0, *minimumClasses >= 0
	if lengthGiven != classesGiven {
		fmt.Fprintln(streams.stderr,
			"a policy change needs both --minimum-length and --minimum-classes")
		return 2
	}

	remote, err := newRemoteAdmin(options, streams)
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	if !lengthGiven {
		return remote.read(ctx, passwordPolicyPath, renderPasswordPolicy)
	}
	document, err := remote.client.Do(ctx, http.MethodPut, passwordPolicyPath, map[string]any{
		"minimum_length":            *minimumLength,
		"minimum_character_classes": *minimumClasses,
	})
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	return remote.show(document, renderPasswordPolicy)
}

func renderPasswordPolicy(stdout io.Writer, document []byte) error {
	var policy struct {
		MinimumLength           *int `json:"minimum_length"`
		MinimumCharacterClasses *int `json:"minimum_character_classes"`
	}
	if err := json.Unmarshal(document, &policy); err != nil {
		return err
	}
	if policy.MinimumLength == nil || policy.MinimumCharacterClasses == nil {
		return fmt.Errorf("the appliance returned an incomplete password policy")
	}
	if _, err := fmt.Fprintf(stdout, "minimum length            %d bytes\n", *policy.MinimumLength); err != nil {
		return err
	}
	_, err := fmt.Fprintf(stdout, "minimum character classes %d of lowercase, uppercase, digit and other\n",
		*policy.MinimumCharacterClasses)
	return err
}
