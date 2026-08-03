package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// backupTables lists what a snapshot contains, in an order that satisfies the
// foreign keys when rows are inserted top to bottom. Restore walks it backwards
// to clear the database, so a child table is always emptied before its parent.
//
// Deliberately absent: events, node_usage, user_usage, node_metrics,
// notification_deliveries. Those are history — they are large, they are pruned
// on a schedule anyway, and nobody restoring a panel needs last month's
// heartbeat samples back. A backup is for the configuration you cannot
// reconstruct, not for the numbers that regenerate themselves.
var backupTables = []string{
	"panel_settings",
	"admins",
	"api_tokens",
	"config_profiles",
	"config_profile_inbounds",
	"nodes",
	"hosts",
	"squads",
	"squad_inbounds",
	"users",
	"user_squads",
	"user_devices",
	"notification_channels",
	"announcements",
}

// Snapshot is a portable copy of a panel's configuration.
type Snapshot struct {
	// Schema is the last applied migration. A snapshot taken on a newer schema
	// cannot be trusted to load into an older one: a column that did not exist
	// yet would be dropped in silence, and the operator would find out when the
	// feature that used it stopped working.
	Schema  string `json:"schema"`
	TakenAt string `json:"takenAt"`
	// Tables maps a table name to its rows, each row a column→value object.
	// Column-keyed rather than positional so a snapshot stays readable, and so
	// a reordered column list cannot silently shift values into wrong fields.
	Tables map[string][]map[string]any `json:"tables"`
	Counts map[string]int              `json:"counts"`
	// Types records each column's Postgres type, because JSON cannot tell a
	// text[] from a jsonb array once both are a list of strings — and binding
	// one as the other is rejected. Without this, restore guesses and fails on
	// the first node row.
	Types map[string]map[string]string `json:"types"`
}

// ExportSnapshot reads every configuration table in one transaction, so the
// snapshot is a single consistent moment rather than a smear across the time
// it took to read.
func (s *Store) ExportSnapshot(ctx context.Context) (*Snapshot, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	snap := &Snapshot{
		Tables: map[string][]map[string]any{},
		Counts: map[string]int{},
		Types:  map[string]map[string]string{},
	}

	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(name), '') FROM schema_migrations`).Scan(&snap.Schema); err != nil {
		return nil, mapErr(err)
	}
	if err := tx.QueryRow(ctx, `SELECT NOW()::text`).Scan(&snap.TakenAt); err != nil {
		return nil, mapErr(err)
	}

	for _, table := range backupTables {
		types, err := columnTypes(ctx, tx, table)
		if err != nil {
			return nil, fmt.Errorf("describe %s: %w", table, err)
		}
		snap.Types[table] = types

		rows, err := tx.Query(ctx, `SELECT * FROM `+quoteIdent(table))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", table, mapErr(err))
		}
		out := []map[string]any{}
		for rows.Next() {
			values, err := rows.Values()
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("read %s: %w", table, mapErr(err))
			}
			row := map[string]any{}
			for i, fd := range rows.FieldDescriptions() {
				row[string(fd.Name)] = normaliseValue(values[i])
			}
			out = append(out, row)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("read %s: %w", table, mapErr(err))
		}
		snap.Tables[table] = out
		snap.Counts[table] = len(out)
	}
	return snap, nil
}

// normaliseValue turns a scanned column into something that survives a JSON
// round trip and can be bound straight back on restore.
//
// The awkward case is uuid: pgx hands it back as a [16]byte array, which
// marshals as a list of sixteen numbers and is then rejected on the way in.
// Every foreign key in this schema is a uuid, so getting this wrong does not
// degrade the backup — it makes restore impossible.
func normaliseValue(v any) any {
	switch t := v.(type) {
	case [16]byte:
		return formatUUID(t)
	case []byte:
		// JSONB arrives as bytes; keep it as structure rather than a blob.
		var decoded any
		if json.Unmarshal(t, &decoded) == nil {
			return decoded
		}
		return string(t)
	default:
		return v
	}
}

func formatUUID(b [16]byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 0, 36)
	for i, x := range b {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out = append(out, '-')
		}
		out = append(out, hex[x>>4], hex[x&0x0f])
	}
	return string(out)
}

// ImportSnapshot replaces the current configuration with the snapshot's.
//
// This is destructive by design — a restore that merged would leave a panel in
// a state that is neither the backup nor what was there before, which is the
// worst outcome of the three. It runs in one transaction: either the whole
// snapshot lands or the database is untouched.
func (s *Store) ImportSnapshot(ctx context.Context, snap *Snapshot, currentSchema string) error {
	if snap == nil || len(snap.Tables) == 0 {
		return fmt.Errorf("the snapshot is empty")
	}
	// Refusing a mismatched schema is the whole reason the version is recorded.
	// Loading a snapshot from a different migration state would drop columns
	// that exist on only one side, silently.
	if snap.Schema != "" && currentSchema != "" && snap.Schema != currentSchema {
		return fmt.Errorf("this snapshot was taken on schema %s but the panel is on %s",
			snap.Schema, currentSchema)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return mapErr(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Clear children before parents.
	for i := len(backupTables) - 1; i >= 0; i-- {
		if _, err := tx.Exec(ctx, `DELETE FROM `+quoteIdent(backupTables[i])); err != nil {
			return fmt.Errorf("clear %s: %w", backupTables[i], mapErr(err))
		}
	}

	for _, table := range backupTables {
		for _, row := range snap.Tables[table] {
			if len(row) == 0 {
				continue
			}
			cols := make([]string, 0, len(row))
			for c := range row {
				cols = append(cols, c)
			}
			// Deterministic order so a failure is reproducible.
			sortStrings(cols)

			placeholders := make([]string, len(cols))
			args := make([]any, len(cols))
			quoted := make([]string, len(cols))
			for i, c := range cols {
				placeholders[i] = fmt.Sprintf("$%d", i+1)
				quoted[i] = quoteIdent(c)
				args[i] = encodeValue(row[c], snap.Types[table][c])
			}

			stmt := fmt.Sprintf(`INSERT INTO %s (%s) VALUES (%s)`,
				quoteIdent(table), strings.Join(quoted, ", "), strings.Join(placeholders, ", "))
			if _, err := tx.Exec(ctx, stmt, args...); err != nil {
				return fmt.Errorf("restore %s: %w", table, mapErr(err))
			}
		}
	}
	return mapErr(tx.Commit(ctx))
}

// columnTypes reads the underlying type of each column, so restore can bind
// values the way the column expects rather than the way JSON happened to
// represent them.
func columnTypes(ctx context.Context, tx pgx.Tx, table string) (map[string]string, error) {
	rows, err := tx.Query(ctx, `SELECT column_name, udt_name
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1`, table)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var name, udt string
		if err := rows.Scan(&name, &udt); err != nil {
			return nil, mapErr(err)
		}
		out[name] = udt
	}
	return out, mapErr(rows.Err())
}

// encodeValue turns a decoded JSON value back into something pgx can bind for
// the column it is going into.
//
// A list of strings is ambiguous in JSON: it is either a text[] column or a
// jsonb one, and Postgres rejects each when handed the other's encoding. The
// recorded column type is what settles it.
func encodeValue(v any, udt string) any {
	isArray := strings.HasPrefix(udt, "_")
	isJSON := udt == "json" || udt == "jsonb"

	switch t := v.(type) {
	case []any:
		if isArray {
			// pgx binds a []string straight onto text[].
			out := make([]string, 0, len(t))
			for _, item := range t {
				out = append(out, fmt.Sprint(item))
			}
			return out
		}
		return marshalOrNil(t)
	case map[string]any:
		return marshalOrNil(t)
	case string:
		// An empty jsonb column round-trips as "" and would be rejected as
		// invalid JSON on the way back.
		if isJSON && t == "" {
			return nil
		}
		return t
	default:
		return v
	}
}

func marshalOrNil(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return string(b)
}

// quoteIdent guards the identifiers this file interpolates. They come from a
// constant list and from the database's own column metadata rather than from a
// request, but building SQL by concatenation without quoting is a habit worth
// not having.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// SchemaVersion is the last applied migration, used to stamp and to check
// snapshots.
func (s *Store) SchemaVersion(ctx context.Context) (string, error) {
	var v string
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(MAX(name), '') FROM schema_migrations`).Scan(&v)
	return v, mapErr(err)
}
