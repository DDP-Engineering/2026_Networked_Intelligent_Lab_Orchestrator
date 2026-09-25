package httpapi

import (
	"net/http"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
)

// The appliance password policy is administered here, beside the identities it
// governs. Both routes declare PermissionPasswordPolicyManage in the one route
// table, so this file adds no authorization of its own; what it adds is the
// rule that a replacement carries the whole policy.

// passwordPolicyView is the one representation of the policy this API
// serializes. All five values are always present in both directions, and each
// is a pointer so an absent value in a request is told from a false one.
type passwordPolicyView struct {
	MinimumLength    *int  `json:"minimum_length"`
	RequireLowercase *bool `json:"require_lowercase"`
	RequireUppercase *bool `json:"require_uppercase"`
	RequireDigit     *bool `json:"require_digit"`
	RequireSpecial   *bool `json:"require_special"`
}

func policyViewOf(policy auth.PasswordPolicy) passwordPolicyView {
	length := policy.MinimumLength
	lower, upper := policy.RequireLowercase, policy.RequireUppercase
	digit, special := policy.RequireDigit, policy.RequireSpecial
	return passwordPolicyView{
		MinimumLength:    &length,
		RequireLowercase: &lower,
		RequireUppercase: &upper,
		RequireDigit:     &digit,
		RequireSpecial:   &special,
	}
}

func handleReadPasswordPolicy(service *identity.Service) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		policy, err := service.PasswordPolicy(request.Context())
		if err != nil {
			writeAdministrationFailure(response, err)
			return
		}
		writeJSON(response, http.StatusOK, policyViewOf(policy))
	}
}

// handleReplacePasswordPolicy takes the whole policy or nothing. The fields are
// pointers so that an absent one is told apart from a zero: merging a partial
// body would let a client relax the half it never sent.
func handleReplacePasswordPolicy(service *identity.Service) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		actor, ok := actorFrom(request.Context())
		if !ok {
			writeUnauthorized(response)
			return
		}
		var body passwordPolicyView
		if !decodeBody(response, request, &body, "invalid_password_policy", passwordPolicyShapeMessage) {
			return
		}
		if body.MinimumLength == nil || body.RequireLowercase == nil ||
			body.RequireUppercase == nil || body.RequireDigit == nil || body.RequireSpecial == nil {
			writeRefusal(response, http.StatusBadRequest, "invalid_password_policy", passwordPolicyShapeMessage)
			return
		}
		policy := auth.PasswordPolicy{
			MinimumLength:    *body.MinimumLength,
			RequireLowercase: *body.RequireLowercase,
			RequireUppercase: *body.RequireUppercase,
			RequireDigit:     *body.RequireDigit,
			RequireSpecial:   *body.RequireSpecial,
		}
		if err := service.SetPasswordPolicyAs(request.Context(), actor, policy); err != nil {
			writeAdministrationFailure(response, err)
			return
		}
		writeJSON(response, http.StatusOK, policyViewOf(policy))
	}
}

const passwordPolicyShapeMessage = "the request body must carry minimum_length as a whole number and require_lowercase, require_uppercase, require_digit and require_special as true or false"
