package postgres

import (
	"context"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/google/uuid"
)

const tokenColumns = `uuid, name, token_hash, token_preview, created_by, last_used_at, expires_at, created_at`

func scanToken(row interface{ Scan(...any) error }) (*domain.APIToken, error) {
	var t domain.APIToken
	err := row.Scan(&t.UUID, &t.Name, &t.TokenHash, &t.Preview, &t.CreatedBy,
		&t.LastUsedAt, &t.ExpiresAt, &t.CreatedAt)
	if err != nil {
		return nil, mapErr(err)
	}
	return &t, nil
}

func (s *Store) CreateAPIToken(ctx context.Context, name, hash, preview, createdBy string, expiresAt *time.Time) (*domain.APIToken, error) {
	row := s.pool.QueryRow(ctx, `INSERT INTO api_tokens (uuid, name, token_hash, token_preview, created_by, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING `+tokenColumns,
		uuid.NewString(), name, hash, preview, createdBy, expiresAt)
	return scanToken(row)
}

func (s *Store) ListAPITokens(ctx context.Context) ([]domain.APIToken, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+tokenColumns+` FROM api_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []domain.APIToken{}
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, mapErr(rows.Err())
}

// APITokenByHash resolves a presented token and rejects expired ones.
func (s *Store) APITokenByHash(ctx context.Context, hash string) (*domain.APIToken, error) {
	t, err := scanToken(s.pool.QueryRow(ctx, `SELECT `+tokenColumns+` FROM api_tokens WHERE token_hash=$1`, hash))
	if err != nil {
		return nil, err
	}
	if t.ExpiresAt != nil && t.ExpiresAt.Before(time.Now()) {
		return nil, ErrNotFound
	}
	// Best effort: a failed timestamp update must not fail the request.
	_, _ = s.pool.Exec(ctx, `UPDATE api_tokens SET last_used_at = NOW() WHERE uuid = $1`, t.UUID)
	return t, nil
}

func (s *Store) DeleteAPIToken(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM api_tokens WHERE uuid=$1`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
