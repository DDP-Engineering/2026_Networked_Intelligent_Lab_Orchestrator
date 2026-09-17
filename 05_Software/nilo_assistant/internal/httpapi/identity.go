package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
)

// CookieSecurity states how browser session cookies are protected. It is
// chosen explicitly at start-up and never inferred: a Secure cookie is never
// sent over plain HTTP, so local development needs a deliberate exception
// rather than deployment silently inheriting an insecure default.
type CookieSecurity string

const (
	// CookieDevelopmentLocalhost omits Secure for the plain-HTTP loopback
	// development topology.
	CookieDevelopmentLocalhost CookieSecurity = "development-localhost"
	// CookieHTTPSDeployment marks session cookies Secure.
	CookieHTTPSDeployment CookieSecurity = "https"
)

// ParseCookieSecurity accepts a declared cookie mode and rejects anything else.
func ParseCookieSecurity(value string) (CookieSecurity, error) {
	switch CookieSecurity(strings.ToLower(strings.TrimSpace(value))) {
	case CookieDevelopmentLocalhost:
		return CookieDevelopmentLocalhost, nil
	case CookieHTTPSDeployment:
		return CookieHTTPSDeployment, nil
	default:
		return "", fmt.Errorf("unknown session cookie mode %q: use %q or %q",
			value, CookieDevelopmentLocalhost, CookieHTTPSDeployment)
	}
}

func (security CookieSecurity) secure() bool { return security == CookieHTTPSDeployment }

type actorContextKey struct{}

func actorFrom(ctx context.Context) (identity.Actor, bool) {
	actor, ok := ctx.Value(actorContextKey{}).(identity.Actor)
	return actor, ok
}

// resolveIdentity turns either credential into one actor, and enforces the
// CSRF check that only cookie-authenticated state changes need. A missing or
// unusable credential is 401; the permission check that follows returns 403.
func resolveIdentity(service *identity.Service, security CookieSecurity, next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		actor, ok := authenticateRequest(request, service)
		if !ok {
			writeUnauthorized(response)
			return
		}
		if actor.Credential == identity.CredentialSession && changesState(request.Method) {
			if !actor.MatchesCSRF(request.Header.Get(auth.CSRFHeaderName)) {
				writeForbidden(response)
				return
			}
		}
		next.ServeHTTP(response, request.WithContext(context.WithValue(request.Context(), actorContextKey{}, actor)))
	})
}

func authenticateRequest(request *http.Request, service *identity.Service) (identity.Actor, bool) {
	if plaintext, present := bearerToken(request.Header.Get("Authorization")); present {
		actor, err := service.AuthenticateToken(request.Context(), plaintext)
		return actor, err == nil
	}
	cookie, err := request.Cookie(auth.SessionCookieName)
	if err != nil || cookie.Value == "" {
		return identity.Actor{}, false
	}
	actor, err := service.AuthenticateSession(request.Context(), cookie.Value)
	return actor, err == nil
}

func changesState(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

// authorize gates one route on a single permission.
func authorize(permission auth.Permission, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		actor, ok := actorFrom(request.Context())
		if !ok {
			writeUnauthorized(response)
			return
		}
		if !actor.Has(permission) {
			writeForbidden(response)
			return
		}
		next(response, request)
	})
}

func sessionOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		actor, ok := actorFrom(request.Context())
		if !ok || actor.Credential != identity.CredentialSession {
			writeForbidden(response)
			return
		}
		next(response, request)
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type sessionResponse struct {
	APIVersion  string            `json:"api_version"`
	Username    string            `json:"username"`
	Role        auth.Role         `json:"role"`
	Permissions []auth.Permission `json:"permissions"`
	Credential  string            `json:"credential"`
	CSRFToken   string            `json:"csrf_token,omitempty"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
}

const maximumLoginRequestBytes = 4 << 10

func handleLogin(service *identity.Service, security CookieSecurity) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		var credentials loginRequest
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, maximumLoginRequestBytes))
		if err := decoder.Decode(&credentials); err != nil {
			writeUnauthorized(response)
			return
		}
		result, err := service.Login(request.Context(), credentials.Username, []byte(credentials.Password))
		if err != nil {
			if errors.Is(err, identity.ErrThrottled) {
				writeError(response, http.StatusTooManyRequests, "too_many_attempts", "")
				return
			}
			writeUnauthorized(response)
			return
		}
		http.SetCookie(response, &http.Cookie{
			Name: auth.SessionCookieName, Value: result.Session.Secret, Path: "/",
			HttpOnly: true, Secure: security.secure(), SameSite: http.SameSiteStrictMode,
			Expires: result.Session.ExpiresAt,
		})
		// The CSRF cookie is deliberately readable: page code must echo it in a
		// header, which is exactly what a cross-site request cannot do.
		http.SetCookie(response, &http.Cookie{
			Name: auth.CSRFCookieName, Value: result.Session.CSRFToken, Path: "/",
			HttpOnly: false, Secure: security.secure(), SameSite: http.SameSiteStrictMode,
			Expires: result.Session.ExpiresAt,
		})
		expiry := result.Session.ExpiresAt
		writeJSON(response, http.StatusOK, sessionResponse{
			APIVersion: apiVersion, Username: result.User.Username, Role: result.User.Role,
			Permissions: result.User.Role.Permissions(), Credential: string(identity.CredentialSession),
			CSRFToken: result.Session.CSRFToken, ExpiresAt: &expiry,
		})
	}
}

func handleLogout(service *identity.Service, security CookieSecurity) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		actor, ok := actorFrom(request.Context())
		if !ok || actor.Credential != identity.CredentialSession {
			writeForbidden(response)
			return
		}
		if err := service.Logout(request.Context(), actor.SessionID); err != nil {
			writeError(response, http.StatusInternalServerError, "logout_failed", "")
			return
		}
		for _, name := range []string{auth.SessionCookieName, auth.CSRFCookieName} {
			http.SetCookie(response, &http.Cookie{
				Name: name, Value: "", Path: "/", MaxAge: -1,
				HttpOnly: name == auth.SessionCookieName, Secure: security.secure(),
				SameSite: http.SameSiteStrictMode,
			})
		}
		response.WriteHeader(http.StatusNoContent)
	}
}

func handleCurrentSession(response http.ResponseWriter, request *http.Request) {
	actor, ok := actorFrom(request.Context())
	if !ok {
		writeUnauthorized(response)
		return
	}
	writeJSON(response, http.StatusOK, sessionResponse{
		APIVersion: apiVersion, Username: actor.User.Username, Role: actor.User.Role,
		Permissions: actor.Permissions, Credential: string(actor.Credential),
	})
}

func writeJSON(response http.ResponseWriter, status int, body any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(body)
}

func writeError(response http.ResponseWriter, status int, code, challenge string) {
	response.Header().Set("Content-Type", "application/json")
	if challenge != "" {
		response.Header().Set("WWW-Authenticate", challenge)
	}
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(struct {
		Error string `json:"error"`
	}{Error: code})
}

// writeUnauthorized reports that no usable credential was presented. It never
// says which part failed, so it leaks nothing about which accounts exist.
func writeUnauthorized(response http.ResponseWriter) {
	writeError(response, http.StatusUnauthorized, "unauthorized", "Bearer")
}

// writeForbidden reports that a usable credential lacks the permission. It
// carries no challenge: repeating the same credential will not help.
func writeForbidden(response http.ResponseWriter) {
	writeError(response, http.StatusForbidden, "forbidden", "")
}

func bearerToken(header string) (string, bool) {
	fields := strings.Fields(header)
	if len(fields) != 2 || fields[0] != "Bearer" || fields[1] == "" {
		return "", false
	}
	return fields[1], true
}
