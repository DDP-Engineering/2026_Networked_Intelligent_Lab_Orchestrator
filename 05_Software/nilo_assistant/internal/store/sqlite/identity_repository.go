package sqlite

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
)

// statements is the surface shared by the connection pool, one reserved
// connection and one transaction, so a single repository implementation serves
// both ordinary work and work that must run inside a transaction.
type statements interface {
	ExecContext(ctx context.Context, query string, arguments ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, arguments ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, arguments ...any) *sql.Row
}

// IdentityRepository persists identities, browser sessions and API tokens.
// Only one-way verifiers of session and token secrets are ever written.
type IdentityRepository struct {
	database *DB
	// bound is set only on the repository handed to an administrative
	// transaction, so every statement it runs joins that transaction.
	bound statements
}

// NewIdentityRepository constructs a repository with its database dependency.
func NewIdentityRepository(database *DB) *IdentityRepository {
	return &IdentityRepository{database: database}
}

const userColumns = "id, username, password_hash, role, created_at, updated_at, disabled_at"

func (repository *IdentityRepository) validate() error {
	if repository == nil {
		return errors.New("identity database is required")
	}
	if repository.bound != nil {
		return nil
	}
	if repository.database == nil || repository.database.SQL() == nil {
		return errors.New("identity database is required")
	}
	return nil
}

func (repository *IdentityRepository) execute() statements {
	if repository.bound != nil {
		return repository.bound
	}
	return repository.database.SQL()
}

// CreateUser stores one identity, reporting a duplicate name as a typed error.
func (repository *IdentityRepository) CreateUser(ctx context.Context, user auth.User) error {
	if err := repository.validate(); err != nil {
		return err
	}
	if user.ID == "" || user.Username == "" || user.PasswordVerifier == "" || user.Role == "" {
		return errors.New("identity fields are required")
	}
	_, err := repository.execute().ExecContext(
		ctx,
		"INSERT INTO users (id, username, password_hash, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		user.ID,
		user.Username,
		user.PasswordVerifier,
		string(user.Role),
		formatTimestamp(user.CreatedAt),
		formatTimestamp(user.UpdatedAt),
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return fmt.Errorf("%w: %s", auth.ErrUsernameTaken, user.Username)
		}
		return fmt.Errorf("store identity: %w", err)
	}
	return nil
}

// UserByID resolves one identity by its stable identifier.
func (repository *IdentityRepository) UserByID(ctx context.Context, id string) (auth.User, error) {
	return repository.userWhere(ctx, "id = ?", id)
}

// UserByUsername resolves one identity by its normalized name.
func (repository *IdentityRepository) UserByUsername(ctx context.Context, username string) (auth.User, error) {
	return repository.userWhere(ctx, "username = ?", username)
}

func (repository *IdentityRepository) userWhere(ctx context.Context, condition string, argument any) (auth.User, error) {
	if err := repository.validate(); err != nil {
		return auth.User{}, err
	}
	row := repository.execute().QueryRowContext(
		ctx, "SELECT "+userColumns+" FROM users WHERE "+condition, argument,
	)
	user, err := scanUser(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.User{}, auth.ErrUserNotFound
	}
	return user, err
}

// ListUsers returns every identity ordered by name for a stable listing.
func (repository *IdentityRepository) ListUsers(ctx context.Context) ([]auth.User, error) {
	if err := repository.validate(); err != nil {
		return nil, err
	}
	rows, err := repository.execute().QueryContext(
		ctx, "SELECT "+userColumns+" FROM users ORDER BY username ASC",
	)
	if err != nil {
		return nil, fmt.Errorf("list identities: %w", err)
	}
	defer rows.Close()
	users := make([]auth.User, 0)
	for rows.Next() {
		user, err := scanUser(rows.Scan)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate identities: %w", err)
	}
	return users, nil
}

// CountEnabledAdministrators reports how many administrators can still act.
func (repository *IdentityRepository) CountEnabledAdministrators(ctx context.Context) (int, error) {
	if err := repository.validate(); err != nil {
		return 0, err
	}
	var count int
	if err := repository.execute().QueryRowContext(
		ctx, "SELECT COUNT(*) FROM users WHERE role = ? AND disabled_at IS NULL", string(auth.RoleAdministrator),
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count enabled administrators: %w", err)
	}
	return count, nil
}

// UpdateUserRole records a new role for one identity.
func (repository *IdentityRepository) UpdateUserRole(ctx context.Context, id string, role auth.Role, at time.Time) error {
	return repository.updateUser(
		ctx, "UPDATE users SET role = ?, updated_at = ? WHERE id = ?", string(role), formatTimestamp(at), id,
	)
}

// SetUserDisabled records or clears an identity's disabled time.
func (repository *IdentityRepository) SetUserDisabled(ctx context.Context, id string, disabledAt *time.Time, at time.Time) error {
	var value any
	if disabledAt != nil {
		value = formatTimestamp(*disabledAt)
	}
	return repository.updateUser(
		ctx, "UPDATE users SET disabled_at = ?, updated_at = ? WHERE id = ?", value, formatTimestamp(at), id,
	)
}

// UpdateUserPassword replaces an identity's stored password verifier.
func (repository *IdentityRepository) UpdateUserPassword(ctx context.Context, id, verifier string, at time.Time) error {
	return repository.updateUser(
		ctx, "UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?", verifier, formatTimestamp(at), id,
	)
}

func (repository *IdentityRepository) updateUser(ctx context.Context, statement string, arguments ...any) error {
	if err := repository.validate(); err != nil {
		return err
	}
	result, err := repository.execute().ExecContext(ctx, statement, arguments...)
	if err != nil {
		return fmt.Errorf("update identity: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read identity update result: %w", err)
	}
	if changed == 0 {
		return auth.ErrUserNotFound
	}
	return nil
}

// CreateSession stores one browser session and only the verifiers of its
// secrets. The stored password verifier the login checked is the condition of
// the insert itself, so a password reset or a disable that completes while a
// login is in flight cannot be overtaken by that login's session: the same
// statement that writes the row decides whether the credential still stands.
func (repository *IdentityRepository) CreateSession(
	ctx context.Context, session auth.SessionRecord, verifier auth.SessionVerifier, passwordVerifier string,
) error {
	if err := repository.validate(); err != nil {
		return err
	}
	if session.ID == "" || session.UserID == "" {
		return errors.New("session identity fields are required")
	}
	if passwordVerifier == "" {
		return errors.New("the password verifier the login checked is required")
	}
	result, err := repository.execute().ExecContext(
		ctx,
		`INSERT INTO user_sessions (id, user_id, verifier, csrf_verifier, created_at, expires_at)
		 SELECT ?, ?, ?, ?, ?, ?
		 WHERE EXISTS (SELECT 1 FROM users WHERE id = ? AND password_hash = ? AND disabled_at IS NULL)`,
		session.ID,
		session.UserID,
		verifier[:],
		session.CSRFVerifier[:],
		formatTimestamp(session.CreatedAt),
		formatTimestamp(session.ExpiresAt),
		session.UserID,
		passwordVerifier,
	)
	if err != nil {
		return fmt.Errorf("store browser session: %w", err)
	}
	stored, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read browser session insert result: %w", err)
	}
	if stored == 0 {
		return auth.ErrCredentialSuperseded
	}
	return nil
}

// SessionBySecret resolves a session cookie secret using a constant-time
// verifier comparison over the stored rows.
func (repository *IdentityRepository) SessionBySecret(ctx context.Context, secret string) (auth.SessionRecord, error) {
	if err := repository.validate(); err != nil {
		return auth.SessionRecord{}, err
	}
	candidate := auth.ComputeSessionVerifier(secret)
	rows, err := repository.execute().QueryContext(
		ctx, "SELECT id, user_id, verifier, csrf_verifier, created_at, expires_at, revoked_at FROM user_sessions",
	)
	if err != nil {
		return auth.SessionRecord{}, fmt.Errorf("read browser sessions: %w", err)
	}
	defer rows.Close()

	var matched auth.SessionRecord
	found := false
	for rows.Next() {
		var verifier, csrfVerifier []byte
		var createdAt, expiresAt string
		var revokedAt sql.NullString
		var record auth.SessionRecord
		if err := rows.Scan(&record.ID, &record.UserID, &verifier, &csrfVerifier, &createdAt, &expiresAt, &revokedAt); err != nil {
			return auth.SessionRecord{}, fmt.Errorf("scan browser session: %w", err)
		}
		if subtle.ConstantTimeCompare(candidate[:], verifier) != 1 {
			continue
		}
		copy(record.CSRFVerifier[:], csrfVerifier)
		if record.CreatedAt, err = parseUTCTimestamp(createdAt); err != nil {
			return auth.SessionRecord{}, fmt.Errorf("parse session creation time: %w", err)
		}
		if record.ExpiresAt, err = parseUTCTimestamp(expiresAt); err != nil {
			return auth.SessionRecord{}, fmt.Errorf("parse session expiry: %w", err)
		}
		if revokedAt.Valid {
			revoked, err := parseUTCTimestamp(revokedAt.String)
			if err != nil {
				return auth.SessionRecord{}, fmt.Errorf("parse session revocation time: %w", err)
			}
			record.RevokedAt = &revoked
		}
		matched = record
		found = true
	}
	if err := rows.Err(); err != nil {
		return auth.SessionRecord{}, fmt.Errorf("iterate browser sessions: %w", err)
	}
	if !found {
		return auth.SessionRecord{}, auth.ErrSessionNotFound
	}
	if matched.RevokedAt != nil {
		return auth.SessionRecord{}, auth.ErrSessionRevoked
	}
	return matched, nil
}

// RevokeSession invalidates one active browser session.
func (repository *IdentityRepository) RevokeSession(ctx context.Context, id string, at time.Time) error {
	if err := repository.validate(); err != nil {
		return err
	}
	result, err := repository.execute().ExecContext(
		ctx, "UPDATE user_sessions SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL", formatTimestamp(at), id,
	)
	if err != nil {
		return fmt.Errorf("revoke browser session: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read session revocation result: %w", err)
	}
	if changed == 0 {
		return auth.ErrSessionNotFound
	}
	return nil
}

// RevokeUserSessions invalidates every active session an identity holds.
// Revoking nothing is not an error: an identity may simply have no session.
func (repository *IdentityRepository) RevokeUserSessions(ctx context.Context, userID string, at time.Time) error {
	if err := repository.validate(); err != nil {
		return err
	}
	if _, err := repository.execute().ExecContext(
		ctx,
		"UPDATE user_sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL",
		formatTimestamp(at),
		userID,
	); err != nil {
		return fmt.Errorf("revoke identity browser sessions: %w", err)
	}
	return nil
}

// CreateAPIToken stores one named token's metadata and only its verifier.
func (repository *IdentityRepository) CreateAPIToken(ctx context.Context, token auth.APIToken, verifier auth.TokenVerifier) error {
	if err := repository.validate(); err != nil {
		return err
	}
	if token.ID == "" || token.UserID == "" {
		return errors.New("token identity fields are required")
	}
	var expiresAt any
	if token.ExpiresAt != nil {
		expiresAt = formatTimestamp(*token.ExpiresAt)
	}
	_, err := repository.execute().ExecContext(
		ctx,
		"INSERT INTO mcp_tokens (id, user_id, verifier, name, scopes, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		token.ID,
		token.UserID,
		verifier[:],
		token.Name,
		auth.FormatPermissions(token.Scopes),
		formatTimestamp(token.CreatedAt),
		expiresAt,
	)
	if err != nil {
		return fmt.Errorf("store API token: %w", err)
	}
	return nil
}

// APITokenBySecret resolves a bearer secret using a constant-time verifier
// comparison. Revocation and expiry are reported to the caller rather than
// hidden, so an unusable token is never mistaken for an unknown one here.
func (repository *IdentityRepository) APITokenBySecret(ctx context.Context, secret string) (auth.APIToken, error) {
	if err := repository.validate(); err != nil {
		return auth.APIToken{}, err
	}
	candidate := auth.TokenVerifier(sha256.Sum256([]byte(secret)))
	rows, err := repository.execute().QueryContext(
		ctx, "SELECT id, user_id, verifier, name, scopes, created_at, expires_at, revoked_at FROM mcp_tokens",
	)
	if err != nil {
		return auth.APIToken{}, fmt.Errorf("read API tokens: %w", err)
	}
	defer rows.Close()

	var matched auth.APIToken
	found := false
	for rows.Next() {
		var verifier []byte
		token, err := scanAPIToken(rows.Scan, &verifier)
		if err != nil {
			return auth.APIToken{}, err
		}
		if subtle.ConstantTimeCompare(candidate[:], verifier) != 1 {
			continue
		}
		matched = token
		found = true
	}
	if err := rows.Err(); err != nil {
		return auth.APIToken{}, fmt.Errorf("iterate API tokens: %w", err)
	}
	if !found {
		return auth.APIToken{}, auth.ErrTokenNotFound
	}
	return matched, nil
}

// ListAPITokens returns every token's metadata, never its secret.
func (repository *IdentityRepository) ListAPITokens(ctx context.Context) ([]auth.APIToken, error) {
	if err := repository.validate(); err != nil {
		return nil, err
	}
	rows, err := repository.execute().QueryContext(
		ctx,
		"SELECT id, user_id, verifier, name, scopes, created_at, expires_at, revoked_at FROM mcp_tokens ORDER BY created_at ASC, id ASC",
	)
	if err != nil {
		return nil, fmt.Errorf("list API tokens: %w", err)
	}
	defer rows.Close()
	tokens := make([]auth.APIToken, 0)
	for rows.Next() {
		var verifier []byte
		token, err := scanAPIToken(rows.Scan, &verifier)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate API tokens: %w", err)
	}
	return tokens, nil
}

// RevokeAPIToken records the revocation time for one active token.
func (repository *IdentityRepository) RevokeAPIToken(ctx context.Context, id string, at time.Time) error {
	if err := repository.validate(); err != nil {
		return err
	}
	result, err := repository.execute().ExecContext(
		ctx, "UPDATE mcp_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL", formatTimestamp(at), id,
	)
	if err != nil {
		return fmt.Errorf("revoke API token: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read token revocation result: %w", err)
	}
	if changed == 0 {
		return auth.ErrTokenNotFound
	}
	return nil
}

// WithinAdministrativeTransaction runs one administrative change and the audit
// event it owes as a single database transaction. The transaction begins with
// `BEGIN IMMEDIATE`, so it takes the database's write lock before it reads
// anything: a guard such as the last-enabled-administrator count cannot be
// split from the change it protects by another connection or another process,
// and a rejected audit insertion takes the change down with it.
func (repository *IdentityRepository) WithinAdministrativeTransaction(
	ctx context.Context, apply func(identity.Repository, audit.Repository) error,
) error {
	if err := repository.validate(); err != nil {
		return err
	}
	if repository.bound != nil {
		return errors.New("an administrative transaction cannot be nested")
	}
	connection, err := repository.database.SQL().Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve the administrative connection: %w", err)
	}
	defer func() { _ = connection.Close() }()
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin the administrative transaction: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		// The rollback runs even for a cancelled context or a panicking change,
		// because this connection returns to the pool either way and must never
		// carry an open transaction back to it.
		_, _ = connection.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
	}()
	if err := apply(
		&IdentityRepository{database: repository.database, bound: connection},
		&AuditRepository{database: repository.database, bound: connection},
	); err != nil {
		return err
	}
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit the administrative transaction: %w", err)
	}
	committed = true
	return nil
}

func scanUser(scan func(...any) error) (auth.User, error) {
	var user auth.User
	var role, createdAt, updatedAt string
	var disabledAt sql.NullString
	if err := scan(&user.ID, &user.Username, &user.PasswordVerifier, &role, &createdAt, &updatedAt, &disabledAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return auth.User{}, err
		}
		return auth.User{}, fmt.Errorf("scan identity: %w", err)
	}
	user.Role = auth.Role(role)
	var err error
	if user.CreatedAt, err = parseUTCTimestamp(createdAt); err != nil {
		return auth.User{}, fmt.Errorf("parse identity creation time: %w", err)
	}
	if user.UpdatedAt, err = parseUTCTimestamp(updatedAt); err != nil {
		return auth.User{}, fmt.Errorf("parse identity update time: %w", err)
	}
	if disabledAt.Valid {
		disabled, err := parseUTCTimestamp(disabledAt.String)
		if err != nil {
			return auth.User{}, fmt.Errorf("parse identity disable time: %w", err)
		}
		user.DisabledAt = &disabled
	}
	return user, nil
}

func scanAPIToken(scan func(...any) error, verifier *[]byte) (auth.APIToken, error) {
	var token auth.APIToken
	var scopes, createdAt string
	var expiresAt, revokedAt sql.NullString
	if err := scan(&token.ID, &token.UserID, verifier, &token.Name, &scopes, &createdAt, &expiresAt, &revokedAt); err != nil {
		return auth.APIToken{}, fmt.Errorf("scan API token: %w", err)
	}
	parsed, err := auth.ParsePermissionList(scopes)
	if err != nil {
		return auth.APIToken{}, fmt.Errorf("parse stored token scopes: %w", err)
	}
	token.Scopes = parsed
	if token.CreatedAt, err = parseUTCTimestamp(createdAt); err != nil {
		return auth.APIToken{}, fmt.Errorf("parse token creation time: %w", err)
	}
	if expiresAt.Valid && strings.TrimSpace(expiresAt.String) != "" {
		expiry, err := parseUTCTimestamp(expiresAt.String)
		if err != nil {
			return auth.APIToken{}, fmt.Errorf("parse token expiry: %w", err)
		}
		token.ExpiresAt = &expiry
	}
	if revokedAt.Valid {
		revoked, err := parseUTCTimestamp(revokedAt.String)
		if err != nil {
			return auth.APIToken{}, fmt.Errorf("parse token revocation time: %w", err)
		}
		token.RevokedAt = &revoked
	}
	return token, nil
}

func formatTimestamp(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
