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
// serializes. Both fields are always present in both directions.
type passwordPolicyView struct {
	MinimumLength           *int `json:"minimum_length"`
	MinimumCharacterClasses *int `json:"minimum_character_classes"`
}

func policyViewOf(policy auth.PasswordPolicy) passwordPolicyView {
	length, classes := policy.MinimumLength, policy.MinimumCharacterClasses
	return passwordPolicyView{MinimumLength: &length, MinimumCharacterClasses: &classes}
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
		if body.MinimumLength == nil || body.MinimumCharacterClasses == nil {
			writeRefusal(response, http.StatusBadRequest, "invalid_password_policy", passwordPolicyShapeMessage)
			return
		}
		policy := auth.PasswordPolicy{
			MinimumLength:           *body.MinimumLength,
			MinimumCharacterClasses: *body.MinimumCharacterClasses,
		}
		if err := service.SetPasswordPolicyAs(request.Context(), actor, policy); err != nil {
			writeAdministrationFailure(response, err)
			return
		}
		writeJSON(response, http.StatusOK, policyViewOf(policy))
	}
}

const passwordPolicyShapeMessage = "the request body must carry both minimum_length and minimum_character_classes as whole numbers"
