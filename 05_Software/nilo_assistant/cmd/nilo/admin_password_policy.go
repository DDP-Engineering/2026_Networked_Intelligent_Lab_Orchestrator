package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	requiredClasses := flags.String("require", "",
		"character classes every password must carry: a comma list of lower, upper, digit and special, or none")
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
	// `--require none` is how an operator states "no class required", so
	// clearing the requirements is typed rather than achieved by omission.
	lengthGiven, classesGiven := *minimumLength != 0, *requiredClasses != ""
	if lengthGiven != classesGiven {
		fmt.Fprintln(streams.stderr,
			"a policy change needs both --minimum-length and --require")
		return 2
	}
	required, err := parseRequiredClasses(*requiredClasses)
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
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
	body := map[string]any{"minimum_length": *minimumLength}
	for field, on := range required {
		body[field] = on
	}
	document, err := remote.client.Do(ctx, http.MethodPut, passwordPolicyPath, body)
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	return remote.show(document, renderPasswordPolicy)
}

// passwordPolicyClasses maps the names an operator types onto the wire fields,
// in the order they are printed. The command knows the vocabulary; the
// appliance keeps the rule.
var passwordPolicyClasses = []struct {
	name  string
	field string
}{
	{"lower", "require_lowercase"},
	{"upper", "require_uppercase"},
	{"digit", "require_digit"},
	{"special", "require_special"},
}

// parseRequiredClasses reads the comma list. An unknown name is refused here,
// before the appliance is contacted, because a misspelt class would otherwise
// be sent as "not required" and quietly weaken the policy the operator meant.
func parseRequiredClasses(value string) (map[string]bool, error) {
	required := map[string]bool{}
	for _, class := range passwordPolicyClasses {
		required[class.field] = false
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "none" {
		return required, nil
	}
	for _, name := range strings.Split(trimmed, ",") {
		name = strings.TrimSpace(name)
		field := ""
		for _, class := range passwordPolicyClasses {
			if class.name == name {
				field = class.field
			}
		}
		if field == "" {
			return nil, fmt.Errorf(
				"unknown password class %q: use lower, upper, digit, special, or none", name)
		}
		required[field] = true
	}
	return required, nil
}

func renderPasswordPolicy(stdout io.Writer, document []byte) error {
	var policy struct {
		MinimumLength    *int  `json:"minimum_length"`
		RequireLowercase *bool `json:"require_lowercase"`
		RequireUppercase *bool `json:"require_uppercase"`
		RequireDigit     *bool `json:"require_digit"`
		RequireSpecial   *bool `json:"require_special"`
	}
	if err := json.Unmarshal(document, &policy); err != nil {
		return err
	}
	if policy.MinimumLength == nil || policy.RequireLowercase == nil ||
		policy.RequireUppercase == nil || policy.RequireDigit == nil || policy.RequireSpecial == nil {
		return fmt.Errorf("the appliance returned an incomplete password policy")
	}
	if _, err := fmt.Fprintf(stdout, "minimum length   %d bytes\n", *policy.MinimumLength); err != nil {
		return err
	}
	required := make([]string, 0, len(passwordPolicyClasses))
	for _, class := range passwordPolicyClasses {
		on := map[string]bool{
			"require_lowercase": *policy.RequireLowercase,
			"require_uppercase": *policy.RequireUppercase,
			"require_digit":     *policy.RequireDigit,
			"require_special":   *policy.RequireSpecial,
		}[class.field]
		if on {
			required = append(required, class.name)
		}
	}
	if len(required) == 0 {
		_, err := fmt.Fprintln(stdout, "required classes none")
		return err
	}
	_, err := fmt.Fprintf(stdout, "required classes %s\n", strings.Join(required, ", "))
	return err
}
