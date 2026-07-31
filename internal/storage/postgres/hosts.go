package postgres

import (
	"context"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/google/uuid"
)

const hostColumns = `h.uuid, h.inbound_uuid, i.tag, i.type, i.config_profile_uuid, COALESCE(p.name, ''),
	h.remark, h.address, h.port, h.path, h.sni, h.host_header, h.alpn, h.fingerprint, h.public_key,
	h.short_id, h.spider_x, h.flow, h.security, h.allow_insecure, h.tags, h.is_disabled,
	h.view_position, h.created_at, h.updated_at`

const hostFrom = ` FROM hosts h
	JOIN config_profile_inbounds i ON i.uuid = h.inbound_uuid
	JOIN config_profiles p ON p.uuid = i.config_profile_uuid`

func scanHost(row interface{ Scan(...any) error }) (*domain.Host, error) {
	var h domain.Host
	err := row.Scan(&h.UUID, &h.InboundID, &h.InboundTag, &h.InboundType, &h.ProfileID, &h.ProfileName,
		&h.Remark, &h.Address, &h.Port, &h.Path, &h.SNI, &h.HostHeader, &h.ALPN, &h.Fingerprint,
		&h.PublicKey, &h.ShortID, &h.SpiderX, &h.Flow, &h.Security, &h.AllowInsecure, &h.Tags,
		&h.IsDisabled, &h.ViewPosition, &h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return nil, mapErr(err)
	}
	if h.Tags == nil {
		h.Tags = []string{}
	}
	return &h, nil
}

type HostInput struct {
	InboundID     string
	Remark        string
	Address       string
	Port          int
	Path          string
	SNI           string
	HostHeader    string
	ALPN          string
	Fingerprint   string
	PublicKey     string
	ShortID       string
	SpiderX       string
	Flow          string
	Security      string
	AllowInsecure bool
	Tags          []string
	IsDisabled    bool
	ViewPosition  int
}

func (s *Store) CreateHost(ctx context.Context, in HostInput) (*domain.Host, error) {
	id := uuid.NewString()
	_, err := s.pool.Exec(ctx, `INSERT INTO hosts
		(uuid, inbound_uuid, remark, address, port, path, sni, host_header, alpn, fingerprint,
		 public_key, short_id, spider_x, flow, security, allow_insecure, tags, is_disabled, view_position)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		id, in.InboundID, in.Remark, in.Address, in.Port, in.Path, in.SNI, in.HostHeader, in.ALPN,
		in.Fingerprint, in.PublicKey, in.ShortID, in.SpiderX, in.Flow, in.Security, in.AllowInsecure,
		in.Tags, in.IsDisabled, in.ViewPosition)
	if err != nil {
		return nil, mapErr(err)
	}
	return s.Host(ctx, id)
}

func (s *Store) UpdateHost(ctx context.Context, id string, in HostInput) (*domain.Host, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE hosts SET inbound_uuid=$2, remark=$3, address=$4, port=$5, path=$6,
		sni=$7, host_header=$8, alpn=$9, fingerprint=$10, public_key=$11, short_id=$12, spider_x=$13,
		flow=$14, security=$15, allow_insecure=$16, tags=$17, is_disabled=$18, view_position=$19, updated_at=NOW()
		WHERE uuid=$1`,
		id, in.InboundID, in.Remark, in.Address, in.Port, in.Path, in.SNI, in.HostHeader, in.ALPN,
		in.Fingerprint, in.PublicKey, in.ShortID, in.SpiderX, in.Flow, in.Security, in.AllowInsecure,
		in.Tags, in.IsDisabled, in.ViewPosition)
	if err != nil {
		return nil, mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return s.Host(ctx, id)
}

func (s *Store) Host(ctx context.Context, id string) (*domain.Host, error) {
	return scanHost(s.pool.QueryRow(ctx, `SELECT `+hostColumns+hostFrom+` WHERE h.uuid = $1`, id))
}

func (s *Store) ListHosts(ctx context.Context) ([]domain.Host, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+hostColumns+hostFrom+` ORDER BY h.view_position, h.created_at`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []domain.Host{}
	for rows.Next() {
		h, err := scanHost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *h)
	}
	return out, mapErr(rows.Err())
}

// HostsForInbounds returns the enabled hosts published by any of the inbounds,
// which is how a user's subscription is assembled from their squads.
func (s *Store) HostsForInbounds(ctx context.Context, inboundIDs []string) ([]domain.Host, error) {
	if len(inboundIDs) == 0 {
		return []domain.Host{}, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT `+hostColumns+hostFrom+`
		WHERE h.inbound_uuid = ANY($1) AND h.is_disabled = FALSE
		ORDER BY h.view_position, h.created_at`, inboundIDs)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []domain.Host{}
	for rows.Next() {
		h, err := scanHost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *h)
	}
	return out, mapErr(rows.Err())
}

func (s *Store) DeleteHost(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM hosts WHERE uuid = $1`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ReorderHosts(ctx context.Context, order []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return mapErr(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for i, id := range order {
		if _, err := tx.Exec(ctx, `UPDATE hosts SET view_position=$2 WHERE uuid=$1`, id, i); err != nil {
			return mapErr(err)
		}
	}
	return mapErr(tx.Commit(ctx))
}
