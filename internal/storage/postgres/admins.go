package postgres

import (
	"context"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/google/uuid"
)

const adminColumns = `uuid, username, password_hash, role, is_disabled, last_login_at, created_at, updated_at,
	coalesce(totp_secret, ''), totp_enabled, totp_confirmed_at, totp_last_step, recovery_code_hashes`

func scanAdmin(row interface{ Scan(...any) error }) (*domain.Admin, error) {
	var a domain.Admin
	err := row.Scan(&a.UUID, &a.Username, &a.PasswordHash, &a.Role, &a.IsDisabled, &a.LastLoginAt, &a.CreatedAt, &a.UpdatedAt,
		&a.TOTPSecret, &a.TOTPEnabled, &a.TOTPConfirmedAt, &a.TOTPLastStep, &a.RecoveryCodeHashes)
	if err != nil {
		return nil, mapErr(err)
	}
	a.RecoveryCodesLeft = len(a.RecoveryCodeHashes)
	return &a, nil
}

func (s *Store) CreateAdmin(ctx context.Context, username, passwordHash string, role domain.AdminRole) (*domain.Admin, error) {
	row := s.pool.QueryRow(ctx, `INSERT INTO admins (uuid, username, password_hash, role)
		VALUES ($1, $2, $3, $4) RETURNING `+adminColumns,
		uuid.NewString(), username, passwordHash, role)
	return scanAdmin(row)
}

func (s *Store) AdminByUsername(ctx context.Context, username string) (*domain.Admin, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+adminColumns+` FROM admins WHERE lower(username) = lower($1)`, username)
	return scanAdmin(row)
}

func (s *Store) AdminByUUID(ctx context.Context, id string) (*domain.Admin, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+adminColumns+` FROM admins WHERE uuid = $1`, id)
	return scanAdmin(row)
}

func (s *Store) ListAdmins(ctx context.Context) ([]domain.Admin, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+adminColumns+` FROM admins ORDER BY created_at`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []domain.Admin{}
	for rows.Next() {
		a, err := scanAdmin(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, mapErr(rows.Err())
}

func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM admins`).Scan(&n)
	return n, mapErr(err)
}

func (s *Store) UpdateAdminPassword(ctx context.Context, id, passwordHash string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE admins SET password_hash = $2, updated_at = NOW() WHERE uuid = $1`, id, passwordHash)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateAdmin(ctx context.Context, id string, role domain.AdminRole, disabled bool) (*domain.Admin, error) {
	row := s.pool.QueryRow(ctx, `UPDATE admins SET role = $2, is_disabled = $3, updated_at = NOW()
		WHERE uuid = $1 RETURNING `+adminColumns, id, role, disabled)
	return scanAdmin(row)
}

func (s *Store) DeleteAdmin(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM admins WHERE uuid = $1`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) TouchAdminLogin(ctx context.Context, id string) {
	_, _ = s.pool.Exec(ctx, `UPDATE admins SET last_login_at = $2 WHERE uuid = $1`, id, time.Now())
}

// ------------------------------------------------------------ two-factor

// StageTOTPSecret stores a secret that is not in force yet. Enrolment is two
// steps on purpose: a secret written straight to the account would lock an
// operator out if they scanned the QR into an app that never actually worked.
func (s *Store) StageTOTPSecret(ctx context.Context, id, secret string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE admins
		SET totp_secret = $2, totp_enabled = false, totp_confirmed_at = NULL, updated_at = NOW()
		WHERE uuid = $1`, id, secret)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ConfirmTOTP turns a staged secret on, once a code proved the app has it.
func (s *Store) ConfirmTOTP(ctx context.Context, id string, step int64, recoveryHashes []string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE admins
		SET totp_enabled = true, totp_confirmed_at = NOW(), totp_last_step = $2,
		    recovery_code_hashes = $3, updated_at = NOW()
		WHERE uuid = $1 AND totp_secret IS NOT NULL`, id, step, recoveryHashes)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DisableTOTP clears the second factor and every recovery code with it, so a
// re-enrolment cannot be unlocked by a code from the previous one.
func (s *Store) DisableTOTP(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE admins
		SET totp_secret = NULL, totp_enabled = false, totp_confirmed_at = NULL,
		    totp_last_step = 0, recovery_code_hashes = '{}', updated_at = NOW()
		WHERE uuid = $1`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ClaimTOTPStep records a time step as used, and reports whether it was still
// unused. The comparison and the write are one statement so two requests racing
// with the same code cannot both win.
func (s *Store) ClaimTOTPStep(ctx context.Context, id string, step int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE admins SET totp_last_step = $2
		WHERE uuid = $1 AND totp_last_step < $2`, id, step)
	if err != nil {
		return false, mapErr(err)
	}
	return tag.RowsAffected() == 1, nil
}

// ConsumeRecoveryCode removes one digest, and reports whether it was there.
// Removal is what makes a recovery code single-use.
func (s *Store) ConsumeRecoveryCode(ctx context.Context, id, digest string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE admins
		SET recovery_code_hashes = array_remove(recovery_code_hashes, $2), updated_at = NOW()
		WHERE uuid = $1 AND $2 = ANY(recovery_code_hashes)`, id, digest)
	if err != nil {
		return false, mapErr(err)
	}
	return tag.RowsAffected() == 1, nil
}

// ReplaceRecoveryCodes swaps the whole set, for regenerating them.
func (s *Store) ReplaceRecoveryCodes(ctx context.Context, id string, hashes []string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE admins SET recovery_code_hashes = $2, updated_at = NOW()
		WHERE uuid = $1 AND totp_enabled`, id, hashes)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
