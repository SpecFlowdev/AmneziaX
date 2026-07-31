package postgres

import (
	"context"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/google/uuid"
)

func (s *Store) CreateSquad(ctx context.Context, name, info string, inboundIDs []string) (*domain.Squad, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id := uuid.NewString()
	if _, err := tx.Exec(ctx, `INSERT INTO squads (uuid, name, info) VALUES ($1,$2,$3)`, id, name, info); err != nil {
		return nil, mapErr(err)
	}
	for _, in := range inboundIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO squad_inbounds (squad_uuid, inbound_uuid) VALUES ($1,$2)
			ON CONFLICT DO NOTHING`, id, in); err != nil {
			return nil, mapErr(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, mapErr(err)
	}
	return s.Squad(ctx, id)
}

func (s *Store) UpdateSquad(ctx context.Context, id, name, info string, inboundIDs []string) (*domain.Squad, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `UPDATE squads SET name=$2, info=$3, updated_at=NOW() WHERE uuid=$1`, id, name, info)
	if err != nil {
		return nil, mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM squad_inbounds WHERE squad_uuid=$1`, id); err != nil {
		return nil, mapErr(err)
	}
	for _, in := range inboundIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO squad_inbounds (squad_uuid, inbound_uuid) VALUES ($1,$2)
			ON CONFLICT DO NOTHING`, id, in); err != nil {
			return nil, mapErr(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, mapErr(err)
	}
	return s.Squad(ctx, id)
}

func (s *Store) Squad(ctx context.Context, id string) (*domain.Squad, error) {
	var sq domain.Squad
	row := s.pool.QueryRow(ctx, `SELECT uuid, name, info, created_at, updated_at FROM squads WHERE uuid=$1`, id)
	if err := row.Scan(&sq.UUID, &sq.Name, &sq.Info, &sq.CreatedAt, &sq.UpdatedAt); err != nil {
		return nil, mapErr(err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM user_squads WHERE squad_uuid=$1`, id).Scan(&sq.MemberCount); err != nil {
		return nil, mapErr(err)
	}
	inbounds, err := s.squadInbounds(ctx, id)
	if err != nil {
		return nil, err
	}
	sq.Inbounds = inbounds
	sq.InboundIDs = make([]string, 0, len(inbounds))
	for _, in := range inbounds {
		sq.InboundIDs = append(sq.InboundIDs, in.UUID)
	}
	return &sq, nil
}

func (s *Store) squadInbounds(ctx context.Context, squadID string) ([]domain.ConfigProfileInbound, error) {
	rows, err := s.pool.Query(ctx, `SELECT i.uuid, i.config_profile_uuid, p.name, i.tag, i.type, i.network, i.security, i.port
		FROM squad_inbounds si
		JOIN config_profile_inbounds i ON i.uuid = si.inbound_uuid
		JOIN config_profiles p ON p.uuid = i.config_profile_uuid
		WHERE si.squad_uuid = $1
		ORDER BY p.name, i.tag`, squadID)
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

func (s *Store) ListSquads(ctx context.Context) ([]domain.Squad, error) {
	rows, err := s.pool.Query(ctx, `SELECT s.uuid, s.name, s.info, s.created_at, s.updated_at,
		(SELECT COUNT(*) FROM user_squads us WHERE us.squad_uuid = s.uuid)
		FROM squads s ORDER BY s.name`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []domain.Squad{}
	for rows.Next() {
		var sq domain.Squad
		if err := rows.Scan(&sq.UUID, &sq.Name, &sq.Info, &sq.CreatedAt, &sq.UpdatedAt, &sq.MemberCount); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, sq)
	}
	if err := rows.Err(); err != nil {
		return nil, mapErr(err)
	}
	for i := range out {
		inbounds, err := s.squadInbounds(ctx, out[i].UUID)
		if err != nil {
			return nil, err
		}
		out[i].Inbounds = inbounds
		out[i].InboundIDs = make([]string, 0, len(inbounds))
		for _, in := range inbounds {
			out[i].InboundIDs = append(out[i].InboundIDs, in.UUID)
		}
	}
	return out, nil
}

func (s *Store) DeleteSquad(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM squads WHERE uuid=$1`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AddAllUsersToSquad and RemoveAllUsersFromSquad back the bulk membership
// actions offered on the squad page.
func (s *Store) AddAllUsersToSquad(ctx context.Context, squadID string) (int64, error) {
	tag, err := s.pool.Exec(ctx, `INSERT INTO user_squads (user_uuid, squad_uuid)
		SELECT u.uuid, $1 FROM users u ON CONFLICT DO NOTHING`, squadID)
	if err != nil {
		return 0, mapErr(err)
	}
	return tag.RowsAffected(), nil
}

func (s *Store) RemoveAllUsersFromSquad(ctx context.Context, squadID string) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM user_squads WHERE squad_uuid=$1`, squadID)
	if err != nil {
		return 0, mapErr(err)
	}
	return tag.RowsAffected(), nil
}
