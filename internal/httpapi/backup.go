package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/SpecFlowdev/AmneziaX/internal/storage/postgres"
)

// postgresSnapshot mirrors the store's snapshot shape at the HTTP boundary, so
// an uploaded file is decoded into a known structure before it reaches the
// database rather than being passed through as whatever JSON arrived.
type postgresSnapshot struct {
	Schema  string                       `json:"schema"`
	TakenAt string                       `json:"takenAt"`
	Tables  map[string][]map[string]any  `json:"tables"`
	Counts  map[string]int               `json:"counts"`
	Types   map[string]map[string]string `json:"types"`
}

func (s postgresSnapshot) toStore() *postgres.Snapshot {
	return &postgres.Snapshot{
		Schema:  s.Schema,
		TakenAt: s.TakenAt,
		Tables:  s.Tables,
		Counts:  s.Counts,
		Types:   s.Types,
	}
}

// maxSnapshotBytes caps an uploaded restore. A panel with a lot of subscribers
// still produces a small file; anything past this is a mistake or an attempt to
// exhaust memory, and neither should be decoded.
const maxSnapshotBytes = 64 << 20

// exportBackup streams the panel's configuration as a downloadable file.
//
// Owner-only, and worth being blunt about why: the snapshot contains every
// subscription uuid, every admin password hash, node enrolment tokens and
// webhook signing secrets. It is the panel, not a report about it.
func (a *API) exportBackup(w http.ResponseWriter, r *http.Request) {
	snap, err := a.store.ExportSnapshot(r.Context())
	if err != nil {
		a.storeErr(w, err)
		return
	}

	a.store.LogEvent(r.Context(), domain.EventSettingsUpdated, claimsOf(r).Username, "",
		"configuration exported", map[string]any{"schema": snap.Schema})

	name := fmt.Sprintf("amneziax-backup-%s.json", time.Now().UTC().Format("2006-01-02-1504"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(snap); err != nil {
		a.log.Error("write backup", "error", err)
	}
}

// backupSummary lets the UI show what a snapshot would replace before anyone
// commits to replacing it.
func (a *API) backupSummary(w http.ResponseWriter, r *http.Request) {
	snap, err := a.store.ExportSnapshot(r.Context())
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema": snap.Schema,
		"counts": snap.Counts,
	})
}

// importBackup replaces the configuration with an uploaded snapshot.
func (a *API) importBackup(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxSnapshotBytes+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "could not read the upload")
		return
	}
	if len(body) > maxSnapshotBytes {
		writeErr(w, http.StatusRequestEntityTooLarge, "that file is too large to be a snapshot")
		return
	}

	var snap postgresSnapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		writeErr(w, http.StatusBadRequest, "this file is not an AmneziaX snapshot")
		return
	}
	if len(snap.Tables) == 0 {
		writeErr(w, http.StatusBadRequest, "this snapshot contains no data")
		return
	}

	current, err := a.store.SchemaVersion(r.Context())
	if err != nil {
		a.storeErr(w, err)
		return
	}

	if err := a.store.ImportSnapshot(r.Context(), snap.toStore(), current); err != nil {
		// A refused restore is nearly always a mismatched schema or a foreign
		// key, and the operator needs the actual reason to act on it.
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	a.invalidateSettings()
	a.store.LogEvent(r.Context(), domain.EventSettingsUpdated, claimsOf(r).Username, "",
		"configuration restored from a snapshot", map[string]any{"schema": snap.Schema})

	writeJSON(w, http.StatusOK, map[string]any{
		"restored": snap.Counts,
		"schema":   snap.Schema,
	})
}
