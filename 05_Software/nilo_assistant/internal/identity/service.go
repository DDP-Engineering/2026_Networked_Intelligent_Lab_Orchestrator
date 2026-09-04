// Package identity owns human identities, browser sessions, API tokens, and
// the authorization decision that gates every protected NILO interface.
//
// It answers one question: may this actor *request* this action. It does not
// answer whether an integration operation may run without a human approval —
// that stays with the integration app's operation policy, and no role and no
// token scope here can weaken it.
package identity

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/google/uuid"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/domain"
)

var (
	// ErrInvalidCredentials is returned for every unusable credential, so a
	// caller never learns which part of the credential failed.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrThrottled reports that this username has exhausted its login attempts.
	ErrThrottled = errors.New("too many failed login attempts")
	// ErrAdministratorExists reports that bootstrap is no longer available.
	ErrAdministratorExists = errors.New("an enabled administrator already exists")
	// ErrLastAdministrator reports a change that would leave nobody able to administer.
	ErrLastAdministrator = errors.New("the last enabled administrator must remain an enabled administrator")
	// ErrNotAdministrator reports that the named actor may not administer identities.
	ErrNotAdministrator = errors.New("actor is not an enabled administrator")
	// ErrExpiryNotInFuture reports a token expiry that has already passed, which
	// would create a credential that is unusable the moment it is disclosed.
	ErrExpiryNotInFuture = errors.New("token expiry must be in the future")
	// ErrUserHasTokens reports a deletion refused because the identity still
	// owns API tokens. Removing them with the identity would destroy credential
	// records nobody decided to destroy, so the tokens are deleted deliberately
	// first and this refusal says so.
	ErrUserHasTokens = errors.New("the identity still owns API tokens")
)

// Repository is the persistence this package needs, named at the point of use.
type Repository interface {
	CreateUser(ctx context.Context, user auth.User) error
	UserByID(ctx context.Context, id string) (auth.User, error)
	UserByUsername(ctx context.Context, username string) (auth.User, error)
	ListUsers(ctx context.Context) ([]auth.User, error)
	CountEnabledAdministrators(ctx context.Context) (int, error)
	UpdateUserRole(ctx context.Context, id string, role auth.Role, at time.Time) error
	SetUserDisabled(ctx context.Context, id string, disabledAt *time.Time, at time.Time) error
	UpdateUserPassword(ctx context.Context, id, verifier string, at time.Time) error
	// DeleteUser removes the identity row itself. There is no disabled-and-kept
	// variant of it: the caller has already decided the record goes.
	DeleteUser(ctx context.Context, id string) error

	// CreateSession stores the session only while the identity still carries the
	// password verifier the login checked and is still enabled.
	CreateSession(
		ctx context.Context, session auth.SessionRecord, verifier auth.SessionVerifier, passwordVerifier string,
	) error
	SessionBySecret(ctx context.Context, secret string) (auth.SessionRecord, error)
	RevokeSession(ctx context.Context, id string, at time.Time) error
	RevokeUserSessions(ctx context.Context, userID string, at time.Time) error
	// DeleteUserSessions removes every session row an identity holds, which is
	// what revocation cannot do for an identity that is itself being removed.
	DeleteUserSessions(ctx context.Context, userID string) error

	CreateAPIToken(ctx context.Context, token auth.APIToken, verifier auth.TokenVerifier) error
	APITokenBySecret(ctx context.Context, secret string) (auth.APIToken, error)
	ListAPITokens(ctx context.Context) ([]auth.APIToken, error)
	RevokeAPIToken(ctx context.Context, id string, at time.Time) error
	// APITokenByID resolves one token's metadata by its identifier, so a
	// deletion can describe what it removed without ever touching a verifier.
	APITokenByID(ctx context.Context, id string) (auth.APIToken, error)
	// DeleteAPIToken removes the token row itself, revoked or not.
	DeleteAPIToken(ctx context.Context, id string) error
	// CountAPITokensForUser counts every token an identity owns, revoked and
	// expired ones included: a revoked token is still a credential record that
	// names its owner, and losing it silently would lose that history.
	CountAPITokensForUser(ctx context.Context, userID string) (int, error)

	// WithinAdministrativeTransaction runs one administrative change and the
	// audit event it owes as a single database transaction, and gives the change
	// the repositories bound to that transaction. Two guarantees follow: an
	// invariant such as the last enabled administrator is read and enforced
	// without another connection or another process changing it in between, and
	// a change whose audit event cannot be written is not kept.
	WithinAdministrativeTransaction(ctx context.Context, apply func(Repository, audit.Repository) error) error
}

// CredentialKind names how an actor proved its identity.
type CredentialKind string

const (
	CredentialSession CredentialKind = "session"
	CredentialToken   CredentialKind = "token"
)

// Actor is one authenticated identity together with the permissions it holds
// for this request. Permissions are resolved from stored state on every
// request, so a role change needs no re-issue and invalidates no cache.
type Actor struct {
	User         auth.User
	Credential   CredentialKind
	SessionID    string
	TokenID      string
	CSRFVerifier auth.SessionVerifier
	Permissions  []auth.Permission
}

// UserActor adapts a locally authenticated administrator to the same rich
// actor boundary used by HTTP requests. Local administration is attributed to
// the user identity, never to a synthetic session or token.
func UserActor(user auth.User) Actor {
	return Actor{User: user, Credential: CredentialSession}
}

// Has reports whether the actor holds permission for this request.
func (actor Actor) Has(permission auth.Permission) bool {
	for _, candidate := range actor.Permissions {
		if candidate == permission {
			return true
		}
	}
	return false
}

// LoginResult is the one-time outcome of a successful password login.
type LoginResult struct {
	Session auth.CreatedSession
	User    auth.User
}

// TokenIssue is the one-time outcome of creating an API token.
type TokenIssue struct {
	Token         auth.APIToken
	Secret        string
	OwnerUsername string
}

// TokenView is one listed token with its owner resolved. It deliberately has
// no field that could carry a secret.
type TokenView struct {
	Token         auth.APIToken
	OwnerUsername string
}

// Options are the explicit dependencies and policy values of the service.
// There is no separate audit dependency: an administrative change and its audit
// event are one transaction, and the repository supplies both halves of it.
type Options struct {
	Repository         Repository
	Now                func() time.Time
	Random             io.Reader
	SessionLifetime    time.Duration
	LoginFailureLimit  int
	LoginFailureWindow time.Duration
	// Audit records login outcomes and composes the complete envelope used by
	// administrative transactions. The transaction still owns the append.
	Audit Journal
}

// Journal records ordinary outcomes and composes events for transaction-owned
// appends. A failed login has no identity and is recorded as anonymous.
type Journal interface {
	Record(ctx context.Context, record audit.Record) error
	Compose(record audit.Record) (audit.Event, error)
}

// Service enforces the identity invariants once, for every interface.
type Service struct {
	repository      Repository
	now             func() time.Time
	random          io.Reader
	sessionLifetime time.Duration
	throttle        *auth.LoginThrottle
	auditJournal    Journal
	dummyVerifier   string
}

// NewService validates the dependencies a correct authorization decision needs.
func NewService(options Options) (*Service, error) {
	if options.Repository == nil {
		return nil, errors.New("identity repository is required")
	}
	if options.Now == nil {
		return nil, errors.New("identity clock is required")
	}
	if options.Audit == nil {
		return nil, errors.New("identity audit journal is required")
	}
	if options.Random == nil {
		options.Random = rand.Reader
	}
	if options.SessionLifetime <= 0 {
		return nil, errors.New("positive session lifetime is required")
	}
	// An unknown username is verified against this fixed verifier so that the
	// response time does not reveal whether the account exists.
	dummyVerifier, err := auth.HashPassword([]byte("nilo-absent-identity-placeholder"), options.Random)
	if err != nil {
		return nil, fmt.Errorf("prepare identity timing placeholder: %w", err)
	}
	return &Service{
		repository:      options.Repository,
		now:             func() time.Time { return options.Now().UTC() },
		auditJournal:    options.Audit,
		random:          options.Random,
		sessionLifetime: options.SessionLifetime,
		throttle:        auth.NewLoginThrottle(options.LoginFailureLimit, options.LoginFailureWindow),
		dummyVerifier:   dummyVerifier,
	}, nil
}

// Bootstrap creates the first administrator. It is the only unauthenticated
// way to obtain administrative rights, so it closes permanently as soon as one
// enabled administrator exists.
func (service *Service) Bootstrap(ctx context.Context, username string, password []byte) (auth.User, error) {
	prepared, err := service.prepareUser(username, auth.RoleAdministrator, password)
	if err != nil {
		return auth.User{}, err
	}
	if err := service.repository.WithinAdministrativeTransaction(
		ctx, func(repository Repository, events audit.Repository) error {
			administrators, err := repository.CountEnabledAdministrators(ctx)
			if err != nil {
				return err
			}
			if administrators > 0 {
				return ErrAdministratorExists
			}
			if err := repository.CreateUser(ctx, prepared); err != nil {
				return err
			}
			// Bootstrap has no prior actor, so the created administrator
			// attributes its own creation.
			return service.record(ctx, events, UserActor(prepared), "identity.user.created", "user", prepared.ID, map[string]string{
				"username": prepared.Username, "role": string(prepared.Role), "origin": "bootstrap",
			})
		},
	); err != nil {
		return auth.User{}, err
	}
	return prepared, nil
}

// CreateUser adds an identity on behalf of an enabled administrator.
func (service *Service) CreateUser(
	ctx context.Context, actor auth.User, username string, role auth.Role, password []byte,
) (auth.User, error) {
	return service.CreateUserAs(ctx, UserActor(actor), username, role, password)
}

// CreateUserAs adds an identity and retains the authenticated credential kind
// for audit attribution.
func (service *Service) CreateUserAs(
	ctx context.Context, actor Actor, username string, role auth.Role, password []byte,
) (auth.User, error) {
	prepared, err := service.prepareUser(username, role, password)
	if err != nil {
		return auth.User{}, err
	}
	if err := service.repository.WithinAdministrativeTransaction(
		ctx, func(repository Repository, events audit.Repository) error {
			if err := requireAdministrator(ctx, repository, actor.User); err != nil {
				return err
			}
			if err := repository.CreateUser(ctx, prepared); err != nil {
				return err
			}
			return service.record(ctx, events, actor, "identity.user.created", "user", prepared.ID, map[string]string{
				"username": prepared.Username, "role": string(prepared.Role),
			})
		},
	); err != nil {
		return auth.User{}, err
	}
	return prepared, nil
}

// SetRole changes an identity's role, refusing to demote the last enabled
// administrator.
func (service *Service) SetRole(ctx context.Context, actor auth.User, username string, role auth.Role) error {
	return service.SetRoleAs(ctx, UserActor(actor), username, role)
}

// SetRoleAs changes an identity's role with request credential attribution.
func (service *Service) SetRoleAs(ctx context.Context, actor Actor, username string, role auth.Role) error {
	if _, err := auth.ParseRole(string(role)); err != nil {
		return err
	}
	normalized, err := auth.NormalizeUsername(username)
	if err != nil {
		return err
	}
	return service.repository.WithinAdministrativeTransaction(
		ctx, func(repository Repository, events audit.Repository) error {
			if err := requireAdministrator(ctx, repository, actor.User); err != nil {
				return err
			}
			subject, err := repository.UserByUsername(ctx, normalized)
			if err != nil {
				return err
			}
			if subject.Role == role {
				return nil
			}
			if err := guardLastAdministrator(ctx, repository, subject, role != auth.RoleAdministrator); err != nil {
				return err
			}
			now := service.now()
			if err := repository.UpdateUserRole(ctx, subject.ID, role, now); err != nil {
				return err
			}
			return service.record(ctx, events, actor, "identity.user.role_changed", "user", subject.ID, map[string]string{
				"username": subject.Username, "previous_role": string(subject.Role), "role": string(role),
			})
		},
	)
}

// SetEnabled disables or re-enables an identity. Disabling also invalidates
// every browser session it holds, because a credential that keeps working
// after revocation has not been revoked.
func (service *Service) SetEnabled(ctx context.Context, actor auth.User, username string, enabled bool) error {
	return service.SetEnabledAs(ctx, UserActor(actor), username, enabled)
}

// SetEnabledAs changes enabled state with request credential attribution.
func (service *Service) SetEnabledAs(ctx context.Context, actor Actor, username string, enabled bool) error {
	normalized, err := auth.NormalizeUsername(username)
	if err != nil {
		return err
	}
	return service.repository.WithinAdministrativeTransaction(
		ctx, func(repository Repository, events audit.Repository) error {
			if err := requireAdministrator(ctx, repository, actor.User); err != nil {
				return err
			}
			subject, err := repository.UserByUsername(ctx, normalized)
			if err != nil {
				return err
			}
			if subject.Enabled() == enabled {
				return nil
			}
			if err := guardLastAdministrator(ctx, repository, subject, !enabled); err != nil {
				return err
			}
			now := service.now()
			var disabledAt *time.Time
			action := "identity.user.enabled"
			if !enabled {
				disabledAt = &now
				action = "identity.user.disabled"
			}
			if err := repository.SetUserDisabled(ctx, subject.ID, disabledAt, now); err != nil {
				return err
			}
			if !enabled {
				if err := repository.RevokeUserSessions(ctx, subject.ID, now); err != nil {
					return err
				}
			}
			return service.record(ctx, events, actor, action, "user", subject.ID, map[string]string{
				"username": subject.Username,
			})
		},
	)
}

// ResetPassword replaces an identity's password and invalidates its sessions.
// A login that verified the replaced password but has not yet stored its
// session cannot complete either: the session insert is conditional on the
// verifier this transaction replaces.
func (service *Service) ResetPassword(ctx context.Context, actor auth.User, username string, password []byte) error {
	return service.ResetPasswordAs(ctx, UserActor(actor), username, password)
}

// ResetPasswordAs resets a password with request credential attribution.
func (service *Service) ResetPasswordAs(ctx context.Context, actor Actor, username string, password []byte) error {
	normalized, err := auth.NormalizeUsername(username)
	if err != nil {
		return err
	}
	verifier, err := auth.HashPassword(password, service.random)
	if err != nil {
		return err
	}
	if err := service.repository.WithinAdministrativeTransaction(
		ctx, func(repository Repository, events audit.Repository) error {
			if err := requireAdministrator(ctx, repository, actor.User); err != nil {
				return err
			}
			subject, err := repository.UserByUsername(ctx, normalized)
			if err != nil {
				return err
			}
			now := service.now()
			if err := repository.UpdateUserPassword(ctx, subject.ID, verifier, now); err != nil {
				return err
			}
			if err := repository.RevokeUserSessions(ctx, subject.ID, now); err != nil {
				return err
			}
			return service.record(ctx, events, actor, "identity.user.password_reset", "user", subject.ID, map[string]string{
				"username": subject.Username,
			})
		},
	); err != nil {
		return err
	}
	// The in-process brake is released only once the reset is committed.
	service.throttle.Reset(normalized)
	return nil
}

// DeleteUser permanently removes an identity.
func (service *Service) DeleteUser(ctx context.Context, actor auth.User, username string) error {
	return service.DeleteUserAs(ctx, UserActor(actor), username)
}

// DeleteUserAs permanently removes an identity with request credential
// attribution.
//
// This is the one identity change that cannot be undone, so everything it has
// to be sure of is decided inside the one transaction that applies it: that an
// enabled administrator is asking, that the appliance keeps an enabled
// administrator afterwards, that no API token is left naming an owner that no
// longer exists, and that the event which records the removal is stored. The
// identity's browser sessions are removed with it, because a session row that
// outlived its identity is a credential nobody can account for.
func (service *Service) DeleteUserAs(ctx context.Context, actor Actor, username string) error {
	normalized, err := auth.NormalizeUsername(username)
	if err != nil {
		// An unusable name is still an attempt. The input itself stays out of
		// the record: it is not a username, and what it is instead is unknown.
		return service.refuseDeletion(ctx, err, actor, "identity.user.delete_refused", "username", "invalid",
			map[string]string{"reason": "invalid_username"})
	}
	err = service.repository.WithinAdministrativeTransaction(
		ctx, func(repository Repository, events audit.Repository) error {
			if err := requireAdministrator(ctx, repository, actor.User); err != nil {
				return err
			}
			subject, err := repository.UserByUsername(ctx, normalized)
			if err != nil {
				return err
			}
			// Removing an identity removes an administrator as surely as
			// demoting or disabling one does, and is guarded by the same count
			// read inside the same transaction.
			if err := guardLastAdministrator(ctx, repository, subject, true); err != nil {
				return err
			}
			// Owned tokens are refused rather than swept along. A cascade here
			// would delete credential records, and their history, on the
			// strength of a decision that was about a person.
			owned, err := repository.CountAPITokensForUser(ctx, subject.ID)
			if err != nil {
				return err
			}
			if owned > 0 {
				return fmt.Errorf("%w: %s", ErrUserHasTokens, subject.Username)
			}
			if err := repository.DeleteUserSessions(ctx, subject.ID); err != nil {
				return err
			}
			if err := repository.DeleteUser(ctx, subject.ID); err != nil {
				return err
			}
			// The label and the usernames keep the record readable once the
			// row is gone; the actor's name is stored for the same reason.
			return service.recordLabeled(ctx, events, actor, "identity.user.deleted", "user", subject.ID, subject.Username, map[string]string{
				"username": subject.Username, "role": string(subject.Role),
				"enabled": strconv.FormatBool(subject.Enabled()), "actor_username": actor.User.Username,
			})
		},
	)
	if reason := deletionRefusalReason(err); reason != "" {
		return service.refuseDeletion(ctx, err, actor, "identity.user.delete_refused", "username", normalized,
			map[string]string{"reason": reason, "username": normalized})
	}
	return err
}

// ListUsers returns every identity, enabled or not.
func (service *Service) ListUsers(ctx context.Context) ([]auth.User, error) {
	return service.repository.ListUsers(ctx)
}

// UserByUsername resolves a stored identity by its normalized name.
func (service *Service) UserByUsername(ctx context.Context, username string) (auth.User, error) {
	normalized, err := auth.NormalizeUsername(username)
	if err != nil {
		return auth.User{}, err
	}
	return service.repository.UserByUsername(ctx, normalized)
}

// Administrator resolves the identity a local administration command names as
// its actor, and refuses anything that is not an enabled administrator.
func (service *Service) Administrator(ctx context.Context, username string) (auth.User, error) {
	user, err := service.UserByUsername(ctx, username)
	if err != nil {
		return auth.User{}, err
	}
	if user.Role != auth.RoleAdministrator || !user.Enabled() {
		return auth.User{}, fmt.Errorf("%w: %s", ErrNotAdministrator, user.Username)
	}
	return user, nil
}

// CreateToken issues one named API token for an owner. The secret is returned
// here and nowhere else; only its verifier is stored.
func (service *Service) CreateToken(
	ctx context.Context, actor auth.User, ownerUsername, name string,
	scopes []auth.Permission, expiresAt *time.Time,
) (TokenIssue, error) {
	return service.CreateTokenAs(ctx, UserActor(actor), ownerUsername, name, scopes, expiresAt)
}

// CreateTokenAs issues a token with request credential attribution.
func (service *Service) CreateTokenAs(
	ctx context.Context, actor Actor, ownerUsername, name string,
	scopes []auth.Permission, expiresAt *time.Time,
) (TokenIssue, error) {
	if name == "" {
		return TokenIssue{}, errors.New("token name is required")
	}
	normalizedOwner, err := auth.NormalizeUsername(ownerUsername)
	if err != nil {
		return TokenIssue{}, err
	}
	for _, scope := range scopes {
		if _, err := auth.ParsePermission(string(scope)); err != nil {
			return TokenIssue{}, err
		}
	}
	if expiresAt != nil && !expiresAt.After(service.now()) {
		return TokenIssue{}, ErrExpiryNotInFuture
	}
	created, verifier, err := auth.NewToken(service.random, service.now())
	if err != nil {
		return TokenIssue{}, err
	}
	var issued TokenIssue
	if err := service.repository.WithinAdministrativeTransaction(
		ctx, func(repository Repository, events audit.Repository) error {
			if err := requireAdministrator(ctx, repository, actor.User); err != nil {
				return err
			}
			owner, err := repository.UserByUsername(ctx, normalizedOwner)
			if err != nil {
				return err
			}
			token := auth.APIToken{
				ID: created.ID, UserID: owner.ID, Name: name, Scopes: scopes,
				CreatedAt: created.CreatedAt, ExpiresAt: expiresAt,
			}
			if err := repository.CreateAPIToken(ctx, token, verifier); err != nil {
				return err
			}
			payload := map[string]string{
				"token_name": name, "owner": owner.Username, "scopes": auth.FormatPermissions(scopes),
			}
			if expiresAt != nil {
				payload["expires_at"] = expiresAt.UTC().Format(time.RFC3339)
			}
			if err := service.record(ctx, events, actor, "identity.token.created", "api_token", token.ID, payload); err != nil {
				return err
			}
			issued = TokenIssue{Token: token, Secret: created.Plaintext, OwnerUsername: owner.Username}
			return nil
		},
	); err != nil {
		return TokenIssue{}, err
	}
	return issued, nil
}

// ListTokens returns every token's metadata with its owner resolved.
func (service *Service) ListTokens(ctx context.Context) ([]TokenView, error) {
	tokens, err := service.repository.ListAPITokens(ctx)
	if err != nil {
		return nil, err
	}
	users, err := service.repository.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	usernames := make(map[string]string, len(users))
	for _, user := range users {
		usernames[user.ID] = user.Username
	}
	views := make([]TokenView, 0, len(tokens))
	for _, token := range tokens {
		views = append(views, TokenView{Token: token, OwnerUsername: usernames[token.UserID]})
	}
	return views, nil
}

// RevokeToken invalidates one API token immediately.
func (service *Service) RevokeToken(ctx context.Context, actor auth.User, tokenID string) error {
	return service.RevokeTokenAs(ctx, UserActor(actor), tokenID)
}

// RevokeTokenAs revokes a token with request credential attribution.
func (service *Service) RevokeTokenAs(ctx context.Context, actor Actor, tokenID string) error {
	return service.repository.WithinAdministrativeTransaction(
		ctx, func(repository Repository, events audit.Repository) error {
			if err := requireAdministrator(ctx, repository, actor.User); err != nil {
				return err
			}
			if err := repository.RevokeAPIToken(ctx, tokenID, service.now()); err != nil {
				return err
			}
			return service.record(ctx, events, actor, "identity.token.revoked", "api_token", tokenID, map[string]string{
				"token_id": tokenID,
			})
		},
	)
}

// DeleteToken permanently removes one API token record.
func (service *Service) DeleteToken(ctx context.Context, actor auth.User, tokenID string) error {
	return service.DeleteTokenAs(ctx, UserActor(actor), tokenID)
}

// DeleteTokenAs permanently removes one API token with request credential
// attribution.
//
// Deletion is deliberately not revocation. Revoking keeps the record and its
// history and stops the secret; deleting removes the record, which also stops
// the secret, because the row a bearer secret is matched against is gone. An
// expired or already revoked token can be deleted too: the record is what is
// being removed, and its usability was settled long before.
func (service *Service) DeleteTokenAs(ctx context.Context, actor Actor, tokenID string) error {
	err := service.repository.WithinAdministrativeTransaction(
		ctx, func(repository Repository, events audit.Repository) error {
			if err := requireAdministrator(ctx, repository, actor.User); err != nil {
				return err
			}
			// Read before delete, so the event can describe what was removed.
			// Nothing read here can carry a secret: the metadata has nowhere to
			// put one and the verifier is never returned.
			token, err := repository.APITokenByID(ctx, tokenID)
			if err != nil {
				return err
			}
			// The owner's name is read now because the record must still read
			// after the owner itself is deleted; a missing owner row is an
			// empty name, not a failure of the deletion.
			owner := ""
			if user, err := repository.UserByID(ctx, token.UserID); err == nil {
				owner = user.Username
			} else if !errors.Is(err, auth.ErrUserNotFound) {
				return err
			}
			if err := repository.DeleteAPIToken(ctx, token.ID); err != nil {
				return err
			}
			expiry, expired := "never", false
			if token.ExpiresAt != nil {
				expiry = token.ExpiresAt.UTC().Format(time.RFC3339)
				expired = !token.ExpiresAt.After(service.now())
			}
			return service.recordLabeled(ctx, events, actor, "identity.token.deleted", "api_token", token.ID, token.Name, map[string]string{
				"token_name": token.Name, "owner_id": token.UserID, "owner": owner,
				"scopes": auth.FormatPermissions(token.Scopes), "expires_at": expiry,
				"revoked": strconv.FormatBool(token.RevokedAt != nil),
				"expired": strconv.FormatBool(expired), "actor_username": actor.User.Username,
			})
		},
	)
	if reason := deletionRefusalReason(err); reason != "" {
		return service.refuseDeletion(ctx, err, actor, "identity.token.delete_refused", "api_token", tokenID,
			map[string]string{"reason": reason, "token_id": tokenID})
	}
	return err
}

// Login verifies a password and establishes one browser session.
func (service *Service) Login(ctx context.Context, username string, password []byte) (LoginResult, error) {
	now := service.now()
	normalized, err := auth.NormalizeUsername(username)
	if err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}
	if !service.throttle.Allow(normalized, now) {
		return LoginResult{}, ErrThrottled
	}
	user, lookupErr := service.repository.UserByUsername(ctx, normalized)
	verifier := service.dummyVerifier
	if lookupErr == nil && user.CanPasswordLogin() {
		verifier = user.PasswordVerifier
	}
	matched, verifyErr := auth.VerifyPassword(verifier, password)
	if lookupErr != nil && !errors.Is(lookupErr, auth.ErrUserNotFound) {
		return LoginResult{}, lookupErr
	}
	if verifyErr != nil || !matched || lookupErr != nil || !user.Enabled() || !user.CanPasswordLogin() {
		service.throttle.RecordFailure(normalized, now)
		service.recordLogin(ctx, audit.Record{
			ActorKind: audit.ActorKindAnonymous, Outcome: audit.OutcomeDenied, Severity: audit.SeverityWarning,
			Action: "identity.login.failed", SubjectType: "username", SubjectID: normalized, TargetLabel: normalized,
		})
		return LoginResult{}, ErrInvalidCredentials
	}

	created, sessionVerifier, csrfVerifier, err := auth.NewSessionSecrets(service.random, now, service.sessionLifetime)
	if err != nil {
		return LoginResult{}, err
	}
	record := auth.SessionRecord{
		ID: created.ID, UserID: user.ID, CSRFVerifier: csrfVerifier,
		CreatedAt: created.CreatedAt, ExpiresAt: created.ExpiresAt,
	}
	// The verifier this login checked is the condition of the insert, so a
	// password reset or a disable that committed while this login was in flight
	// leaves it with no session at all rather than with a session that outlived
	// the credential it was granted for.
	if err := service.repository.CreateSession(ctx, record, sessionVerifier, user.PasswordVerifier); err != nil {
		if errors.Is(err, auth.ErrCredentialSuperseded) {
			return LoginResult{}, ErrInvalidCredentials
		}
		return LoginResult{}, err
	}
	service.throttle.Reset(normalized)
	if actor, err := domain.ParseActorID(user.ID); err == nil {
		service.recordLogin(ctx, audit.Record{
			ActorKind: audit.ActorKindUser, ActorID: actor, Outcome: audit.OutcomeApplied, Severity: audit.SeverityInfo,
			Action: "identity.login.succeeded", SubjectType: "user", SubjectID: user.ID, TargetLabel: user.Username,
			Details: map[string]string{"session_id": created.ID},
		})
	}
	return LoginResult{Session: created, User: user}, nil
}

// recordLogin writes one login outcome where a journal is configured. The
// outcome of the login itself does not depend on the record being written:
// a login is not a change to the appliance, and refusing it would lock the
// administrator out exactly when the store needs attention.
func (service *Service) recordLogin(ctx context.Context, record audit.Record) {
	record.Category, record.Component = audit.CategoryAudit, "identity"
	_ = service.auditJournal.Record(ctx, record)
}

// Logout invalidates one browser session server-side.
func (service *Service) Logout(ctx context.Context, sessionID string) error {
	return service.repository.RevokeSession(ctx, sessionID, service.now())
}

// AuthenticateSession resolves a session cookie secret into an actor, checking
// expiry, revocation and the owner's current state on every request.
func (service *Service) AuthenticateSession(ctx context.Context, secret string) (Actor, error) {
	record, err := service.repository.SessionBySecret(ctx, secret)
	if err != nil {
		if errors.Is(err, auth.ErrSessionNotFound) || errors.Is(err, auth.ErrSessionRevoked) {
			return Actor{}, ErrInvalidCredentials
		}
		return Actor{}, err
	}
	if record.Expired(service.now()) {
		return Actor{}, ErrInvalidCredentials
	}
	user, err := service.repository.UserByID(ctx, record.UserID)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			return Actor{}, ErrInvalidCredentials
		}
		return Actor{}, err
	}
	if !user.Enabled() {
		return Actor{}, ErrInvalidCredentials
	}
	return Actor{
		User: user, Credential: CredentialSession, SessionID: record.ID,
		CSRFVerifier: record.CSRFVerifier, Permissions: user.Role.Permissions(),
	}, nil
}

// AuthenticateToken resolves a bearer secret into an actor whose permissions
// are the token's scopes intersected with its owner's current role.
func (service *Service) AuthenticateToken(ctx context.Context, secret string) (Actor, error) {
	token, err := service.repository.APITokenBySecret(ctx, secret)
	if err != nil {
		if errors.Is(err, auth.ErrTokenNotFound) || errors.Is(err, auth.ErrTokenRevoked) {
			return Actor{}, ErrInvalidCredentials
		}
		return Actor{}, err
	}
	if !token.Usable(service.now()) {
		return Actor{}, ErrInvalidCredentials
	}
	user, err := service.repository.UserByID(ctx, token.UserID)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			return Actor{}, ErrInvalidCredentials
		}
		return Actor{}, err
	}
	if !user.Enabled() {
		return Actor{}, ErrInvalidCredentials
	}
	return Actor{
		User: user, Credential: CredentialToken, TokenID: token.ID,
		Permissions: auth.EffectivePermissions(user.Role, token.Scopes),
	}, nil
}

// MatchesCSRF compares a presented CSRF token against the actor's session in
// constant time.
func (actor Actor) MatchesCSRF(presented string) bool {
	if actor.Credential != CredentialSession || presented == "" {
		return false
	}
	expected := actor.CSRFVerifier
	candidate := auth.ComputeSessionVerifier(presented)
	return subtle.ConstantTimeCompare(expected[:], candidate[:]) == 1
}

// prepareUser builds one identity value outside any transaction, because
// password hashing is deliberately slow and must not hold the database's write
// lock while it runs.
func (service *Service) prepareUser(username string, role auth.Role, password []byte) (auth.User, error) {
	normalized, err := auth.NormalizeUsername(username)
	if err != nil {
		return auth.User{}, err
	}
	if _, err := auth.ParseRole(string(role)); err != nil {
		return auth.User{}, err
	}
	verifier, err := auth.HashPassword(password, service.random)
	if err != nil {
		return auth.User{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return auth.User{}, fmt.Errorf("create identity UUID v7: %w", err)
	}
	now := service.now()
	return auth.User{
		ID: id.String(), Username: normalized, Role: role,
		PasswordVerifier: verifier, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// guardLastAdministrator refuses a change that would remove the final enabled
// administrator, which would leave the appliance unadministrable. It counts
// inside the caller's transaction, so the count it reads is the count the
// change is applied to.
func guardLastAdministrator(
	ctx context.Context, repository Repository, subject auth.User, removesAdministrator bool,
) error {
	if !removesAdministrator || subject.Role != auth.RoleAdministrator || !subject.Enabled() {
		return nil
	}
	administrators, err := repository.CountEnabledAdministrators(ctx)
	if err != nil {
		return err
	}
	if administrators <= 1 {
		return ErrLastAdministrator
	}
	return nil
}

// requireAdministrator re-reads the actor inside the caller's transaction, so a
// role or enabled state that changed since the actor was resolved is seen.
func requireAdministrator(ctx context.Context, repository Repository, actor auth.User) error {
	if actor.ID == "" {
		return ErrNotAdministrator
	}
	current, err := repository.UserByID(ctx, actor.ID)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			return ErrNotAdministrator
		}
		return err
	}
	if current.Role != auth.RoleAdministrator || !current.Enabled() {
		return fmt.Errorf("%w: %s", ErrNotAdministrator, current.Username)
	}
	return nil
}

// record appends one applied audit event through the transaction of the change
// it attributes. Payloads carry names and roles only; no password, token secret
// or verifier ever reaches the audit history.
func (service *Service) record(
	ctx context.Context, events audit.Repository, actor Actor, action, subjectType, subjectID string,
	payload map[string]string,
) error {
	return service.recordLabeled(ctx, events, actor, action, subjectType, subjectID, "", payload)
}

// recordLabeled is record with the subject's display label, for an event that
// has to stay readable after the subject row itself is gone.
func (service *Service) recordLabeled(
	ctx context.Context, events audit.Repository, actor Actor, action, subjectType, subjectID, label string,
	payload map[string]string,
) error {
	actorKind, actorID := actorAttribution(actor)
	parsedActor, err := domain.ParseActorID(actorID)
	if err != nil {
		return fmt.Errorf("parse audit actor %q: %w", actorID, err)
	}
	event, err := service.auditJournal.Compose(audit.Record{
		Category: audit.CategoryAudit, Component: "identity", Action: action,
		SubjectType: subjectType, SubjectID: subjectID, TargetLabel: label,
		ActorKind: actorKind, ActorID: parsedActor,
		Outcome: audit.OutcomeApplied, Severity: audit.SeverityInfo,
		Details: payload,
	})
	if err != nil {
		return err
	}
	return events.Append(ctx, event)
}

// actorAttribution is the audit view of an actor: a session is attributed to
// the user, a bearer request to the token that made it.
func actorAttribution(actor Actor) (audit.ActorKind, string) {
	if actor.Credential == CredentialToken {
		return audit.ActorKindToken, actor.TokenID
	}
	return audit.ActorKindUser, actor.User.ID
}

// deletionRefusalReason maps a refused deletion to its stable reason code. It
// returns "" for anything that is not a refusal but a failure of the store
// itself, which is reported to the caller and is not a decision to record.
func deletionRefusalReason(err error) string {
	switch {
	case errors.Is(err, ErrNotAdministrator):
		return "not_administrator"
	case errors.Is(err, ErrLastAdministrator):
		return "last_administrator"
	case errors.Is(err, ErrUserHasTokens):
		return "user_has_tokens"
	case errors.Is(err, auth.ErrUserNotFound):
		return "user_not_found"
	case errors.Is(err, auth.ErrTokenNotFound):
		return "token_not_found"
	}
	return ""
}

// refuseDeletion records a refused permanent deletion and returns the refusal.
// The refusal stands whether or not the record could be written, because
// nothing changed and the operator has already been told; but a record that
// could not be written is joined to the error, so the caller is never left
// believing the attempt is in the history when it is not. errors.Is still
// finds the refusal reason through the join, so the HTTP mapping and the
// command line report the same code as before.
func (service *Service) refuseDeletion(
	ctx context.Context, refusal error, actor Actor, action, subjectType, subjectID string, details map[string]string,
) error {
	if err := service.recordDeletionRefusal(ctx, actor, action, subjectType, subjectID, details); err != nil {
		return errors.Join(refusal, fmt.Errorf("record the refused deletion: %w", err))
	}
	return refusal
}

// recordDeletionRefusal writes one denied record once the transaction that
// refused a permanent deletion has rolled back. A refusal is a decision an
// investigator needs as much as an applied removal, so it names the actor, the
// requested subject and the reason. The requested subject is the label,
// because a subject that was never found has no other stable name; a request
// that named no subject at all is recorded against "unknown".
func (service *Service) recordDeletionRefusal(
	ctx context.Context, actor Actor, action, subjectType, subjectID string, details map[string]string,
) error {
	actorKind, actorID := actorAttribution(actor)
	parsedActor, err := domain.ParseActorID(actorID)
	if err != nil {
		return fmt.Errorf("attribute the refusal to actor %q: %w", actorID, err)
	}
	if subjectID == "" {
		subjectID = "unknown"
	}
	details["actor_username"] = actor.User.Username
	return service.auditJournal.Record(ctx, audit.Record{
		Category: audit.CategoryAudit, Component: "identity", Action: action,
		SubjectType: subjectType, SubjectID: subjectID, TargetLabel: subjectID,
		ActorKind: actorKind, ActorID: parsedActor,
		Outcome: audit.OutcomeDenied, Severity: audit.SeverityWarning,
		Details: details,
	})
}
