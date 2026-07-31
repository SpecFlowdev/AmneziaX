package postgres

import (
	"context"
	"encoding/json"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/google/uuid"
)

func (s *Store) CreateProfile(ctx context.Context, name string, config json.RawMessage, inbounds []domain.ConfigProfileInbound) (*domain.ConfigProfile, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id := uuid.NewString()
	var p domain.ConfigProfile
	row := tx.QueryRow(ctx, `INSERT INTO config_profiles (uuid, name, config) VALUES ($1, $2, $3)
		RETURNING uuid, name, config, created_at, updated_at`, id, name, config)
	if err := row.Scan(&p.UUID, &p.Name, &p.Config, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, mapErr(err)
	}
	for _, in := range inbounds {
		if _, err := tx.Exec(ctx, `INSERT INTO config_profile_inbounds (uuid, config_profile_uuid, tag, type, network, security, port)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			uuid.NewString(), id, in.Tag, in.Type, in.Network, in.Security, in.Port); err != nil {
			return nil, mapErr(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, mapErr(err)
	}
	p.Inbounds, err = s.ProfileInbounds(ctx, id)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateProfile rewrites the document and reconciles the extracted inbound rows.
// Inbounds are matched by tag so hosts and squads keep pointing at the same
// inbound identity across edits.
func (s *Store) UpdateProfile(ctx context.Context, id, name string, config json.RawMessage, inbounds []domain.ConfigProfileInbound) (*domain.ConfigProfile, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var p domain.ConfigProfile
	row := tx.QueryRow(ctx, `UPDATE config_profiles SET name = $2, config = $3, updated_at = NOW()
		WHERE uuid = $1 RETURNING uuid, name, config, created_at, updated_at`, id, name, config)
	if err := row.Scan(&p.UUID, &p.Name, &p.Config, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, mapErr(err)
	}

	keep := make([]string, 0, len(inbounds))
	for _, in := range inbounds {
		keep = append(keep, in.Tag)
		if _, err := tx.Exec(ctx, `INSERT INTO config_profile_inbounds (uuid, config_profile_uuid, tag, type, network, security, port)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (config_profile_uuid, tag) DO UPDATE
			SET type = EXCLUDED.type, network = EXCLUDED.network, security = EXCLUDED.security, port = EXCLUDED.port`,
			uuid.NewString(), id, in.Tag, in.Type, in.Network, in.Security, in.Port); err != nil {
			return nil, mapErr(err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM config_profile_inbounds WHERE config_profile_uuid = $1 AND tag <> ALL($2)`, id, keep); err != nil {
		return nil, mapErr(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, mapErr(err)
	}
	p.Inbounds, err = s.ProfileInbounds(ctx, id)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) Profile(ctx context.Context, id string) (*domain.ConfigProfile, error) {
	var p domain.ConfigProfile
	row := s.pool.QueryRow(ctx, `SELECT uuid, name, config, created_at, updated_at FROM config_profiles WHERE uuid = $1`, id)
	if err := row.Scan(&p.UUID, &p.Name, &p.Config, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, mapErr(err)
	}
	var err error
	if p.Inbounds, err = s.ProfileInbounds(ctx, id); err != nil {
		return nil, err
	}
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(array_agg(uuid::text), '{}') FROM nodes WHERE config_profile_uuid = $1`, id).Scan(&p.NodeIDs); err != nil {
		return nil, mapErr(err)
	}
	return &p, nil
}

func (s *Store) ListProfiles(ctx context.Context) ([]domain.ConfigProfile, error) {
	rows, err := s.pool.Query(ctx, `SELECT uuid, name, config, created_at, updated_at FROM config_profiles ORDER BY created_at`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []domain.ConfigProfile{}
	for rows.Next() {
		var p domain.ConfigProfile
		if err := rows.Scan(&p.UUID, &p.Name, &p.Config, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, mapErr(err)
	}

	all, err := s.ProfileInbounds(ctx, "")
	if err != nil {
		return nil, err
	}
	byProfile := map[string][]domain.ConfigProfileInbound{}
	for _, in := range all {
		byProfile[in.ConfigProfileID] = append(byProfile[in.ConfigProfileID], in)
	}
	for i := range out {
		out[i].Inbounds = byProfile[out[i].UUID]
	}
	return out, nil
}

// ProfileInbounds returns the inbounds of one profile, or of every profile when
// profileID is empty.
func (s *Store) ProfileInbounds(ctx context.Context, profileID string) ([]domain.ConfigProfileInbound, error) {
	query := `SELECT i.uuid, i.config_profile_uuid, p.name, i.tag, i.type, i.network, i.security, i.port
		FROM config_profile_inbounds i
		JOIN config_profiles p ON p.uuid = i.config_profile_uuid`
	args := []any{}
	if profileID != "" {
		query += ` WHERE i.config_profile_uuid = $1`
		args = append(args, profileID)
	}
	query += ` ORDER BY p.name, i.tag`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []domain.ConfigProfileInbound{}
	for rows.Next() {
		var in domain.ConfigProfileInbound
		if err := rows.Scan(&in.UUID, &in.ConfigProfileID, &in.ProfileName, &in.Tag, &in.Type, &in.Network, &in.Security, &in.Port); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, in)
	}
	return out, mapErr(rows.Err())
}

func (s *Store) Inbound(ctx context.Context, id string) (*domain.ConfigProfileInbound, error) {
	var in domain.ConfigProfileInbound
	row := s.pool.QueryRow(ctx, `SELECT i.uuid, i.config_profile_uuid, p.name, i.tag, i.type, i.network, i.security, i.port
		FROM config_profile_inbounds i
		JOIN config_profiles p ON p.uuid = i.config_profile_uuid
		WHERE i.uuid = $1`, id)
	if err := row.Scan(&in.UUID, &in.ConfigProfileID, &in.ProfileName, &in.Tag, &in.Type, &in.Network, &in.Security, &in.Port); err != nil {
		return nil, mapErr(err)
	}
	return &in, nil
}

func (s *Store) DeleteProfile(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM config_profiles WHERE uuid = $1`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
