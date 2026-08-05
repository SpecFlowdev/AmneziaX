package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/google/uuid"
)

const userColumns = `uuid, short_uuid, username, subscription_uuid, vless_uuid, trojan_password, ss_password,
	status, traffic_limit_bytes, used_traffic_bytes, lifetime_traffic_bytes, traffic_reset_strategy,
	last_traffic_reset_at, expire_at, online_at, description, tag, email, telegram_id, hwid_device_limit,
	sub_last_user_agent, sub_last_opened_at, sub_revoked_at, wg_private_key, wg_public_key, wg_index,
	created_at, updated_at`

func scanUser(row interface{ Scan(...any) error }) (*domain.User, error) {
	var u domain.User
	err := row.Scan(&u.UUID, &u.ShortUUID, &u.Username, &u.SubscriptionUUID, &u.VlessUUID, &u.TrojanPassword,
		&u.SSPassword, &u.Status, &u.TrafficLimitBytes, &u.UsedTrafficBytes, &u.LifetimeTrafficBytes,
		&u.TrafficResetPolicy, &u.LastTrafficResetAt, &u.ExpireAt, &u.OnlineAt, &u.Description, &u.Tag,
		&u.Email, &u.TelegramID, &u.HWIDDeviceLimit, &u.SubLastUA, &u.SubLastOpenedAt, &u.SubRevokedAt,
		&u.WGPrivateKey, &u.WGPublicKey, &u.WGIndex, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, mapErr(err)
	}
	return &u, nil
}

type UserInput struct {
	Username          string
	Status            domain.UserStatus
	TrafficLimitBytes int64
	TrafficReset      domain.TrafficResetStrategy
	ExpireAt          *time.Time
	Description       string
	Tag               string
	Email             string
	TelegramID        *int64
	HWIDDeviceLimit   int
	SquadIDs          []string
}

type UserSecrets struct {
	ShortUUID        string
	SubscriptionUUID string
	VlessUUID        string
	TrojanPassword   string
	SSPassword       string
	WGPrivateKey     string
	WGPublicKey      string
}

func (s *Store) CreateUser(ctx context.Context, in UserInput, sec UserSecrets) (*domain.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id := uuid.NewString()
	_, err = tx.Exec(ctx, `INSERT INTO users
		(uuid, short_uuid, username, subscription_uuid, vless_uuid, trojan_password, ss_password, status,
		 traffic_limit_bytes, traffic_reset_strategy, expire_at, description, tag, email, telegram_id, hwid_device_limit,
		 wg_private_key, wg_public_key)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		id, sec.ShortUUID, in.Username, sec.SubscriptionUUID, sec.VlessUUID, sec.TrojanPassword, sec.SSPassword,
		in.Status, in.TrafficLimitBytes, in.TrafficReset, in.ExpireAt, in.Description, in.Tag, in.Email,
		in.TelegramID, in.HWIDDeviceLimit, sec.WGPrivateKey, sec.WGPublicKey)
	if err != nil {
		return nil, mapErr(err)
	}
	for _, sq := range in.SquadIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO user_squads (user_uuid, squad_uuid) VALUES ($1,$2) ON CONFLICT DO NOTHING`, id, sq); err != nil {
			return nil, mapErr(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, mapErr(err)
	}
	return s.User(ctx, id)
}

func (s *Store) UpdateUser(ctx context.Context, id string, in UserInput) (*domain.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `UPDATE users SET username=$2, status=$3, traffic_limit_bytes=$4,
		traffic_reset_strategy=$5, expire_at=$6, description=$7, tag=$8, email=$9, telegram_id=$10,
		hwid_device_limit=$11, updated_at=NOW() WHERE uuid=$1`,
		id, in.Username, in.Status, in.TrafficLimitBytes, in.TrafficReset, in.ExpireAt, in.Description,
		in.Tag, in.Email, in.TelegramID, in.HWIDDeviceLimit)
	if err != nil {
		return nil, mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM user_squads WHERE user_uuid=$1`, id); err != nil {
		return nil, mapErr(err)
	}
	for _, sq := range in.SquadIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO user_squads (user_uuid, squad_uuid) VALUES ($1,$2) ON CONFLICT DO NOTHING`, id, sq); err != nil {
			return nil, mapErr(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, mapErr(err)
	}
	return s.User(ctx, id)
}

func (s *Store) User(ctx context.Context, id string) (*domain.User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE uuid=$1`, id))
	if err != nil {
		return nil, err
	}
	return s.withSquads(ctx, u)
}

func (s *Store) UserBySubscription(ctx context.Context, subUUID string) (*domain.User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE subscription_uuid=$1`, subUUID))
	if err != nil {
		return nil, err
	}
	return s.withSquads(ctx, u)
}

func (s *Store) UserByShortUUID(ctx context.Context, short string) (*domain.User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE short_uuid=$1`, short))
	if err != nil {
		return nil, err
	}
	return s.withSquads(ctx, u)
}

func (s *Store) withSquads(ctx context.Context, u *domain.User) (*domain.User, error) {
	rows, err := s.pool.Query(ctx, `SELECT s.uuid, s.name FROM user_squads us
		JOIN squads s ON s.uuid = us.squad_uuid WHERE us.user_uuid=$1 ORDER BY s.name`, u.UUID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	u.SquadIDs = []string{}
	u.Squads = []domain.Squad{}
	for rows.Next() {
		var sq domain.Squad
		if err := rows.Scan(&sq.UUID, &sq.Name); err != nil {
			return nil, mapErr(err)
		}
		u.SquadIDs = append(u.SquadIDs, sq.UUID)
		u.Squads = append(u.Squads, sq)
	}
	return u, mapErr(rows.Err())
}

type UserFilter struct {
	Search  string
	Status  string
	SquadID string
	Tag     string
	Limit   int
	Offset  int
	SortBy  string
	Desc    bool
}

var userSortColumns = map[string]string{
	"username":    "u.username",
	"createdAt":   "u.created_at",
	"expireAt":    "u.expire_at",
	"usedTraffic": "u.used_traffic_bytes",
	"status":      "u.status",
	"onlineAt":    "u.online_at",
}

func (s *Store) ListUsers(ctx context.Context, f UserFilter) ([]domain.User, int, error) {
	where := []string{"TRUE"}
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.Search != "" {
		add("(u.username ILIKE '%%' || $%[1]d || '%%' OR u.email ILIKE '%%' || $%[1]d || '%%' OR u.description ILIKE '%%' || $%[1]d || '%%' OR u.short_uuid ILIKE '%%' || $%[1]d || '%%')", f.Search)
	}
	if f.Status != "" {
		add("u.status = $%d", f.Status)
	}
	if f.Tag != "" {
		add("u.tag = $%d", f.Tag)
	}
	if f.SquadID != "" {
		add("EXISTS (SELECT 1 FROM user_squads us WHERE us.user_uuid = u.uuid AND us.squad_uuid = $%d)", f.SquadID)
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users u WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}

	order := userSortColumns[f.SortBy]
	if order == "" {
		order = "u.created_at"
	}
	dir := "ASC"
	if f.Desc {
		dir = "DESC"
	}
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 50
	}
	args = append(args, f.Limit, f.Offset)
	query := fmt.Sprintf(`SELECT %s FROM users u WHERE %s ORDER BY %s %s NULLS LAST LIMIT $%d OFFSET $%d`,
		userColumns, cond, order, dir, len(args)-1, len(args))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	defer rows.Close()

	out := []domain.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, mapErr(err)
	}
	for i := range out {
		if _, err := s.withSquads(ctx, &out[i]); err != nil {
			return nil, 0, err
		}
	}
	return out, total, nil
}

func (s *Store) DeleteUser(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM users WHERE uuid=$1`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetUserStatus(ctx context.Context, id string, status domain.UserStatus) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET status=$2, updated_at=NOW() WHERE uuid=$1`, id, status)
	return mapErr(err)
}

func (s *Store) ResetUserTraffic(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET used_traffic_bytes=0, last_traffic_reset_at=NOW(),
		status = CASE WHEN status='LIMITED' THEN 'ACTIVE' ELSE status END, updated_at=NOW() WHERE uuid=$1`, id)
	return mapErr(err)
}

func (s *Store) RevokeUserSubscription(ctx context.Context, id, newSubUUID, newShort, newVless, newTrojan, newSS string) (*domain.User, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET subscription_uuid=$2, short_uuid=$3, vless_uuid=$4,
		trojan_password=$5, ss_password=$6, sub_revoked_at=NOW(), updated_at=NOW() WHERE uuid=$1`,
		id, newSubUUID, newShort, newVless, newTrojan, newSS)
	if err != nil {
		return nil, mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return s.User(ctx, id)
}

func (s *Store) TouchSubscriptionOpen(ctx context.Context, id, userAgent string) {
	_, _ = s.pool.Exec(ctx, `UPDATE users SET sub_last_user_agent=$2, sub_last_opened_at=NOW() WHERE uuid=$1`, id, userAgent)
}

// ProvisionedUser is the minimal projection the orchestrator needs to inject a
// client into an xray inbound.
type ProvisionedUser struct {
	UUID           string
	Username       string
	VlessUUID      string
	TrojanPassword string
	SSPassword     string
	InboundTag     string
	WGPublicKey    string
	WGIndex        int64
}

// UsersForNode returns every (user, inbound tag) pair that should be present in
// the config of the given node: active users, in a squad, whose squad grants an
// inbound of the node's profile that the node actually serves.
func (s *Store) UsersForNode(ctx context.Context, profileID string, inboundTags []string) ([]ProvisionedUser, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT u.uuid, u.username, u.vless_uuid, u.trojan_password, u.ss_password, i.tag,
		       u.wg_public_key, u.wg_index
		FROM users u
		JOIN user_squads us ON us.user_uuid = u.uuid
		JOIN squad_inbounds si ON si.squad_uuid = us.squad_uuid
		JOIN config_profile_inbounds i ON i.uuid = si.inbound_uuid
		WHERE u.status = 'ACTIVE'
		  AND i.config_profile_uuid = $1
		  AND i.tag = ANY($2)
		ORDER BY i.tag, u.username`, profileID, inboundTags)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []ProvisionedUser{}
	for rows.Next() {
		var p ProvisionedUser
		if err := rows.Scan(&p.UUID, &p.Username, &p.VlessUUID, &p.TrojanPassword, &p.SSPassword, &p.InboundTag,
			&p.WGPublicKey, &p.WGIndex); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, p)
	}
	return out, mapErr(rows.Err())
}

// UserInboundIDs lists the inbound identities a user can reach through their squads.
func (s *Store) UserInboundIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT si.inbound_uuid FROM user_squads us
		JOIN squad_inbounds si ON si.squad_uuid = us.squad_uuid WHERE us.user_uuid = $1`, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, id)
	}
	return out, mapErr(rows.Err())
}

// AddUserTraffic charges usage to a user and returns the user's post-update
// state so the caller can enforce limits.
func (s *Store) AddUserTraffic(ctx context.Context, userID string, bytes int64) (used, limit int64, status domain.UserStatus, err error) {
	row := s.pool.QueryRow(ctx, `UPDATE users
		SET used_traffic_bytes = used_traffic_bytes + $2,
		    lifetime_traffic_bytes = lifetime_traffic_bytes + $2,
		    online_at = NOW()
		WHERE uuid = $1
		RETURNING used_traffic_bytes, traffic_limit_bytes, status`, userID, bytes)
	err = mapErr(row.Scan(&used, &limit, &status))
	return
}

func (s *Store) MarkUserOnline(ctx context.Context, userID string) {
	_, _ = s.pool.Exec(ctx, `UPDATE users SET online_at = NOW() WHERE uuid = $1`, userID)
}

// ExpireUsers flips past-due active users to EXPIRED and returns how many changed.
func (s *Store) ExpireUsers(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET status='EXPIRED', updated_at=NOW()
		WHERE status='ACTIVE' AND expire_at IS NOT NULL AND expire_at < NOW()`)
	if err != nil {
		return 0, mapErr(err)
	}
	return tag.RowsAffected(), nil
}

// ResetDueUserTraffic applies the per-user reset strategy and returns how many
// counters were rolled over.
func (s *Store) ResetDueUserTraffic(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE users
		SET used_traffic_bytes = 0,
		    last_traffic_reset_at = NOW(),
		    status = CASE WHEN status = 'LIMITED' THEN 'ACTIVE' ELSE status END,
		    updated_at = NOW()
		WHERE traffic_reset_strategy <> 'NO_RESET'
		  AND COALESCE(last_traffic_reset_at, created_at) < NOW() - (
		      CASE traffic_reset_strategy
		        WHEN 'DAY' THEN INTERVAL '1 day'
		        WHEN 'WEEK' THEN INTERVAL '7 days'
		        WHEN 'MONTH' THEN INTERVAL '30 days'
		      END)`)
	if err != nil {
		return 0, mapErr(err)
	}
	return tag.RowsAffected(), nil
}

func (s *Store) UserTags(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT tag FROM users WHERE tag <> '' ORDER BY tag`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, t)
	}
	return out, mapErr(rows.Err())
}

// UsersMissingWireGuardKeys returns the users created before WireGuard existed,
// so the panel can give them a key pair without an operator touching anything.
func (s *Store) UsersMissingWireGuardKeys(ctx context.Context, limit int) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT uuid FROM users WHERE wg_public_key = '' ORDER BY created_at LIMIT $1`, limit)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, id)
	}
	return out, mapErr(rows.Err())
}

// SetWireGuardKeys fills in a key pair, but only where one is not already set —
// so two panels racing on the same database cannot hand the same subscriber two
// different keys, one of which would silently never connect.
func (s *Store) SetWireGuardKeys(ctx context.Context, id, private, public string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET wg_private_key = $2, wg_public_key = $3
		 WHERE uuid = $1 AND wg_public_key = ''`, id, private, public)
	return mapErr(err)
}
