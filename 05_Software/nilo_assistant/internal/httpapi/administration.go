package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
)

// This file is the identity and token administration surface: the routes the
// browser Authentication page drives. It adds no authorization of its own.
// Every handler here is registered behind the same credential resolution and
// the same per-route permission check as every other protected route, and each
// one takes its acting administrator from the request context. No request body
// names an actor, and a body that carries one is ignored rather than honored.

const maximumAdministrationRequestBytes = 8 << 10

// identityView is the only identity representation this API serializes. It has
// no field a password verifier could occupy, which is what keeps a stored
// credential out of a response by construction rather than by review.
type identityView struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Role      auth.Role `json:"role"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// apiTokenView is the only token representation this API serializes, and it
// likewise has nowhere to put a verifier or a secret. Revoked is derived here
// so a client never has to interpret a timestamp to decide usability.
type apiTokenView struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Owner     string     `json:"owner"`
	Scopes    []string   `json:"scopes"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at"`
	Revoked   bool       `json:"revoked"`
	RevokedAt *time.Time `json:"revoked_at"`
}

type userListResponse struct {
	APIVersion string         `json:"api_version"`
	Items      []identityView `json:"items"`
}

type tokenListResponse struct {
	APIVersion string         `json:"api_version"`
	Items      []apiTokenView `json:"items"`
}

type createdIdentityResponse struct {
	APIVersion string `json:"api_version"`
	identityView
}

// tokenIssueResponse is the one response in the whole API that carries a token
// secret, because it is the one moment the secret exists outside the caller.
type tokenIssueResponse struct {
	APIVersion string       `json:"api_version"`
	Token      apiTokenView `json:"token"`
	Secret     string       `json:"secret"`
}

func handleListUsers(service *identity.Service) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		users, err := service.ListUsers(request.Context())
		if err != nil {
			writeInternalError(response)
			return
		}
		items := make([]identityView, 0, len(users))
		for _, user := range users {
			items = append(items, identityRepresentation(user))
		}
		writeJSON(response, http.StatusOK, userListResponse{APIVersion: apiVersion, Items: items})
	}
}

func handleCreateUser(service *identity.Service) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		actor, ok := actorFrom(request.Context())
		if !ok {
			writeUnauthorized(response)
			return
		}
		var body struct {
			Username string `json:"username"`
			Role     string `json:"role"`
			Password string `json:"password"`
		}
		if !decodeBody(response, request, &body, "invalid_request", requestShapeMessage) {
			return
		}
		username, ok := readUsername(response, body.Username)
		if !ok {
			return
		}
		role, ok := readRole(response, body.Role)
		if !ok {
			return
		}
		if !readPassword(response, body.Password) {
			return
		}
		created, err := service.CreateUser(request.Context(), actor.User, username, role, []byte(body.Password))
		if err != nil {
			writeAdministrationFailure(response, err)
			return
		}
		writeJSON(response, http.StatusCreated, createdIdentityResponse{
			APIVersion: apiVersion, identityView: identityRepresentation(created),
		})
	}
}

func handleSetRole(service *identity.Service) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		actor, username, ok := administrativeSubject(response, request)
		if !ok {
			return
		}
		var body struct {
			Role string `json:"role"`
		}
		if !decodeBody(response, request, &body, "invalid_role", roleMessage) {
			return
		}
		role, ok := readRole(response, body.Role)
		if !ok {
			return
		}
		if err := service.SetRole(request.Context(), actor.User, username, role); err != nil {
			writeAdministrationFailure(response, err)
			return
		}
		response.WriteHeader(http.StatusNoContent)
	}
}

func handleSetEnabled(service *identity.Service) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		actor, username, ok := administrativeSubject(response, request)
		if !ok {
			return
		}
		// A pointer, so an absent field and an explicit false are told apart:
		// silently reading a missing field as "disable" is not a mistake this
		// route may make.
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		if !decodeBody(response, request, &body, "invalid_enabled", enabledMessage) {
			return
		}
		if body.Enabled == nil {
			writeRefusal(response, http.StatusBadRequest, "invalid_enabled", enabledMessage)
			return
		}
		if err := service.SetEnabled(request.Context(), actor.User, username, *body.Enabled); err != nil {
			writeAdministrationFailure(response, err)
			return
		}
		response.WriteHeader(http.StatusNoContent)
	}
}

func handleResetPassword(service *identity.Service) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		actor, username, ok := administrativeSubject(response, request)
		if !ok {
			return
		}
		var body struct {
			Password string `json:"password"`
		}
		if !decodeBody(response, request, &body, "invalid_password", passwordMessage()) {
			return
		}
		if !readPassword(response, body.Password) {
			return
		}
		if err := service.ResetPassword(request.Context(), actor.User, username, []byte(body.Password)); err != nil {
			writeAdministrationFailure(response, err)
			return
		}
		// No body at all: there is nothing to report that the caller did not
		// send, and the one thing it did send is a credential.
		response.WriteHeader(http.StatusNoContent)
	}
}

func handleListTokens(service *identity.Service) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		views, err := service.ListTokens(request.Context())
		if err != nil {
			writeInternalError(response)
			return
		}
		items := make([]apiTokenView, 0, len(views))
		for _, view := range views {
			items = append(items, tokenRepresentation(view.Token, view.OwnerUsername))
		}
		writeJSON(response, http.StatusOK, tokenListResponse{APIVersion: apiVersion, Items: items})
	}
}

func handleCreateToken(service *identity.Service) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		actor, ok := actorFrom(request.Context())
		if !ok {
			writeUnauthorized(response)
			return
		}
		var body struct {
			Owner     string   `json:"owner"`
			Name      string   `json:"name"`
			Scopes    []string `json:"scopes"`
			ExpiresAt *string  `json:"expires_at"`
		}
		if !decodeBody(response, request, &body, "invalid_request", requestShapeMessage) {
			return
		}
		owner, ok := readUsername(response, body.Owner)
		if !ok {
			return
		}
		name := strings.TrimSpace(body.Name)
		if name == "" {
			writeRefusal(response, http.StatusBadRequest, "invalid_name", "name must not be empty")
			return
		}
		scopes, ok := readScopes(response, body.Scopes)
		if !ok {
			return
		}
		expiresAt, ok := readExpiry(response, body.ExpiresAt)
		if !ok {
			return
		}
		issued, err := service.CreateToken(request.Context(), actor.User, owner, name, scopes, expiresAt)
		if err != nil {
			writeAdministrationFailure(response, err)
			return
		}
		writeJSON(response, http.StatusCreated, tokenIssueResponse{
			APIVersion: apiVersion,
			Token:      tokenRepresentation(issued.Token, issued.OwnerUsername),
			Secret:     issued.Secret,
		})
	}
}

func handleRevokeToken(service *identity.Service) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		actor, ok := actorFrom(request.Context())
		if !ok {
			writeUnauthorized(response)
			return
		}
		if err := service.RevokeToken(request.Context(), actor.User, request.PathValue("id")); err != nil {
			writeAdministrationFailure(response, err)
			return
		}
		response.WriteHeader(http.StatusNoContent)
	}
}

// administrativeSubject resolves the acting administrator from the request
// context and the subject identity from the path, which are the two things
// every per-identity route needs and neither of which comes from the body.
func administrativeSubject(
	response http.ResponseWriter, request *http.Request,
) (identity.Actor, string, bool) {
	actor, ok := actorFrom(request.Context())
	if !ok {
		writeUnauthorized(response)
		return identity.Actor{}, "", false
	}
	username, ok := readUsername(response, request.PathValue("username"))
	if !ok {
		return identity.Actor{}, "", false
	}
	return actor, username, true
}

func identityRepresentation(user auth.User) identityView {
	return identityView{
		ID: user.ID, Username: user.Username, Role: user.Role,
		Enabled: user.Enabled(), CreatedAt: user.CreatedAt,
	}
}

func tokenRepresentation(token auth.APIToken, owner string) apiTokenView {
	scopes := make([]string, 0, len(token.Scopes))
	for _, scope := range token.Scopes {
		scopes = append(scopes, string(scope))
	}
	return apiTokenView{
		ID: token.ID, Name: token.Name, Owner: owner, Scopes: scopes,
		CreatedAt: token.CreatedAt, ExpiresAt: token.ExpiresAt,
		Revoked: token.RevokedAt != nil, RevokedAt: token.RevokedAt,
	}
}

// decodeBody reads a bounded JSON body. Unknown fields are deliberately
// ignored rather than rejected: an `actor` field a client adds must be inert,
// not a way to make the request fail loudly enough to be worth retrying.
// A body that cannot be read at all is reported with the code the route owns,
// because a route with one field has only one thing a bad shape can mean.
func decodeBody(
	response http.ResponseWriter, request *http.Request, target any, code, message string,
) bool {
	decoder := json.NewDecoder(
		http.MaxBytesReader(response, request.Body, maximumAdministrationRequestBytes),
	)
	if err := decoder.Decode(target); err != nil {
		writeRefusal(response, http.StatusBadRequest, code, message)
		return false
	}
	return true
}

func readUsername(response http.ResponseWriter, value string) (string, bool) {
	normalized, err := auth.NormalizeUsername(value)
	if err != nil {
		writeRefusal(response, http.StatusBadRequest, "invalid_username", usernameMessage)
		return "", false
	}
	return normalized, true
}

func readRole(response http.ResponseWriter, value string) (auth.Role, bool) {
	role, err := auth.ParseRole(value)
	if err != nil {
		writeRefusal(response, http.StatusBadRequest, "invalid_role", roleMessage)
		return "", false
	}
	return role, true
}

func readPassword(response http.ResponseWriter, value string) bool {
	if len(value) < auth.MinPasswordBytes || len(value) > auth.MaxPasswordBytes {
		writeRefusal(response, http.StatusBadRequest, "invalid_password", passwordMessage())
		return false
	}
	return true
}

func readScopes(response http.ResponseWriter, values []string) ([]auth.Permission, bool) {
	if len(values) == 0 {
		return nil, true
	}
	scopes := make([]auth.Permission, 0, len(values))
	for _, value := range values {
		scope, err := auth.ParsePermission(value)
		if err != nil {
			writeRefusal(response, http.StatusBadRequest, "invalid_scope", scopeMessage)
			return nil, false
		}
		scopes = append(scopes, scope)
	}
	return scopes, true
}

// readExpiry parses the optional expiry. Whether the instant is far enough in
// the future is not decided here: that is an invariant of the identity service,
// which owns the clock the rest of the credential model is judged against.
func readExpiry(response http.ResponseWriter, value *string) (*time.Time, bool) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, true
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*value))
	if err != nil {
		writeRefusal(response, http.StatusBadRequest, "invalid_expiry", expiryMessage)
		return nil, false
	}
	utc := parsed.UTC()
	return &utc, true
}

const (
	requestShapeMessage = "the request body must be a JSON object of the documented shape"
	usernameMessage     = "username must be 2 to 64 characters of letters, digits, '-', '_' or '.'"
	roleMessage         = "role must be one of administrator, operator or viewer"
	enabledMessage      = "enabled must be true or false"
	scopeMessage        = "every scope must name a permission this appliance defines"
	expiryMessage       = "expiry must be an RFC 3339 instant in the future"
)

func passwordMessage() string {
	return fmt.Sprintf("password must be between %d and %d bytes", auth.MinPasswordBytes, auth.MaxPasswordBytes)
}

// writeAdministrationFailure maps the identity service's typed failures onto
// the documented client errors. Anything it does not recognize becomes an
// internal error with no message: a message built from an unrecognized error
// would hand a caller the text of this process's internals.
func writeAdministrationFailure(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrUsernameTaken):
		writeRefusal(response, http.StatusConflict, "username_taken",
			"a different identity already uses that username")
	case errors.Is(err, auth.ErrUserNotFound):
		writeRefusal(response, http.StatusNotFound, "user_not_found",
			"no identity carries that username")
	case errors.Is(err, auth.ErrTokenNotFound):
		writeRefusal(response, http.StatusNotFound, "token_not_found",
			"no active API token carries that identifier")
	case errors.Is(err, identity.ErrLastAdministrator):
		writeRefusal(response, http.StatusConflict, "last_administrator",
			"the last enabled administrator must remain an enabled administrator")
	case errors.Is(err, identity.ErrExpiryNotInFuture):
		writeRefusal(response, http.StatusBadRequest, "invalid_expiry", expiryMessage)
	case errors.Is(err, identity.ErrNotAdministrator):
		// The route's permission check already passed, so this means the actor
		// lost its administrator role between authentication and the change.
		writeForbidden(response)
	default:
		writeInternalError(response)
	}
}

// writeRefusal reports a client error the caller can act on. The message is one
// of the fixed strings above; it never echoes a submitted value and never
// carries the text of an internal error.
func writeRefusal(response http.ResponseWriter, status int, code, message string) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(struct {
		Error   string `json:"error"`
		Message string `json:"message,omitempty"`
	}{Error: code, Message: message})
}

func writeInternalError(response http.ResponseWriter) {
	writeError(response, http.StatusInternalServerError, "internal_error", "")
}
