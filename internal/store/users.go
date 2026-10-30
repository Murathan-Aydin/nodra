package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound = errors.New("introuvable")
	ErrConflict = errors.New("existe déjà")
	ErrInUse    = errors.New("encore utilisé")
)

type User struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	Name            string     `json:"name"`
	PasswordHash    string     `json:"-"`
	IsInstanceAdmin bool       `json:"is_instance_admin"`
	EmailVerifiedAt *time.Time `json:"email_verified_at"`
	TOTPSecretEnc   *string    `json:"-"`
	TOTPEnabledAt   *time.Time `json:"-"`
	RecoveryCodes   []string   `json:"-"`
	DisabledAt      *time.Time `json:"disabled_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

func (u *User) TwoFactorEnabled() bool { return u.TOTPEnabledAt != nil }

const userCols = `id::text, email, name, password_hash, is_instance_admin, email_verified_at,
	totp_secret_enc, totp_enabled_at, recovery_codes, disabled_at, created_at`

func scanUser(row pgx.Row) (*User, error) {
	u := &User{}
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.IsInstanceAdmin, &u.EmailVerifiedAt,
		&u.TOTPSecretEnc, &u.TOTPEnabledAt, &u.RecoveryCodes, &u.DisabledAt, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

func isForeignKeyViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23503"
}

// CreateUser crée un utilisateur. Le premier utilisateur de l'instance en devient l'administrateur
// (verrou de table pour éviter deux « premiers » utilisateurs concurrents).
func (s *Store) CreateUser(ctx context.Context, email, name, hash string) (*User, error) {
	var u *User
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `LOCK TABLE users IN SHARE ROW EXCLUSIVE MODE`); err != nil {
			return err
		}
		var err error
		u, err = scanUser(tx.QueryRow(ctx, `
			INSERT INTO users (email, name, password_hash, is_instance_admin)
			VALUES ($1, $2, $3, NOT EXISTS (SELECT 1 FROM users))
			RETURNING `+userCols, email, name, hash))
		if err != nil {
			return err
		}
		// Chaque utilisateur reçoit une équipe personnelle dont il est propriétaire.
		display := name
		if display == "" {
			display, _, _ = strings.Cut(email, "@")
		}
		_, err = createTeamTx(ctx, tx, "Équipe de "+display, "", nil, true, u.ID)
		return err
	})
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	return u, err
}

func (s *Store) UserByEmail(ctx context.Context, email string) (*User, error) {
	return scanUser(s.Pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE lower(email) = lower($1)`, email))
}

func (s *Store) UserByID(ctx context.Context, id string) (*User, error) {
	return scanUser(s.Pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) UpdateUserProfile(ctx context.Context, id, name string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET name = $2, updated_at = now() WHERE id = $1`, id, name)
	return err
}

// SetPassword change le mot de passe et révoque toutes les sessions sauf exceptHash.
func (s *Store) SetPassword(ctx context.Context, id, hash, exceptSessionHash string) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`, id, hash); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1 AND token_hash <> $2`, id, exceptSessionHash)
		return err
	})
}

func (s *Store) SetTOTPSecret(ctx context.Context, id, enc string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET totp_secret_enc = $2, updated_at = now() WHERE id = $1 AND totp_enabled_at IS NULL`, id, enc)
	return err
}

func (s *Store) EnableTOTP(ctx context.Context, id string, recoveryHashes []string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET totp_enabled_at = now(), recovery_codes = $2, updated_at = now() WHERE id = $1`, id, recoveryHashes)
	return err
}

func (s *Store) DisableTOTP(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET totp_secret_enc = NULL, totp_enabled_at = NULL, recovery_codes = '{}', updated_at = now() WHERE id = $1`, id)
	return err
}

// UseRecoveryCode consomme un code de secours ; renvoie false s'il n'existe pas.
func (s *Store) UseRecoveryCode(ctx context.Context, id, codeHash string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE users SET recovery_codes = array_remove(recovery_codes, $2), updated_at = now()
		WHERE id = $1 AND $2 = ANY(recovery_codes)`, id, codeHash)
	return tag.RowsAffected() == 1, err
}

// --- Administration de l'instance ---

// AdminUser : utilisateur vu par l'administrateur de l'instance.
type AdminUser struct {
	User
	TwoFactor  bool       `json:"two_factor_enabled"`
	Teams      int        `json:"teams"`
	LastSeenAt *time.Time `json:"last_seen_at"`
}

// ListUsers liste les utilisateurs (recherche sur l'e-mail ou le nom), du plus récent au plus ancien.
func (s *Store) ListUsers(ctx context.Context, search string, limit int) ([]*AdminUser, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+prefixed(userCols, "u.")+`,
		(SELECT count(*) FROM team_members m WHERE m.user_id = u.id),
		(SELECT max(last_seen_at) FROM sessions x WHERE x.user_id = u.id)
		FROM users u WHERE $1 = '' OR u.email ILIKE '%' || $1 || '%' OR u.name ILIKE '%' || $1 || '%'
		ORDER BY u.created_at DESC LIMIT $2`, search, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*AdminUser{}
	for rows.Next() {
		a := &AdminUser{}
		u := &a.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.IsInstanceAdmin, &u.EmailVerifiedAt, &u.TOTPSecretEnc,
			&u.TOTPEnabledAt, &u.RecoveryCodes, &u.DisabledAt, &u.CreatedAt, &a.Teams, &a.LastSeenAt); err != nil {
			return nil, err
		}
		a.TwoFactor = u.TwoFactorEnabled()
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetUserAdmin donne ou retire le rôle d'administrateur de l'instance.
func (s *Store) SetUserAdmin(ctx context.Context, id string, admin bool) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE users SET is_instance_admin = $2, updated_at = now() WHERE id = $1`, id, admin)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// SetUserDisabled désactive (sessions révoquées, jetons refusés) ou réactive un compte.
func (s *Store) SetUserDisabled(ctx context.Context, id string, disabled bool) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE users SET disabled_at = CASE WHEN $2 THEN coalesce(disabled_at, now()) END, updated_at = now()
			WHERE id = $1`, id, disabled)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if disabled {
			_, err = tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, id)
		}
		return err
	})
}

// DeleteUserSessions déconnecte l'utilisateur partout.
func (s *Store) DeleteUserSessions(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, id)
	return err
}
