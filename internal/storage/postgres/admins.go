package postgres

import (
	"context"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/google/uuid"
)

const adminColumns = `uuid, username, password_hash, role, is_disabled, last_login_at, created_at, updated_at`

func scanAdmin(row interface{ Scan(...any) error }) (*domain.Admin, error) {
	var a domain.Admin
	err := row.Scan(&a.UUID, &a.Username, &a.PasswordHash, &a.Role, &a.IsDisabled, &a.LastLoginAt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, mapErr(err)
	}
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
