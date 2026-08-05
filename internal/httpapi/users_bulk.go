package httpapi

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/SpecFlowdev/AmneziaX/internal/storage/postgres"
)

// Handing out access to a class, an office or a reseller's customers means
// creating dozens of identical users. Doing that one form at a time is the kind
// of work a panel exists to remove.

type bulkCreateRequest struct {
	// Either a prefix and a count, or an explicit list of names. The list wins
	// when both are given, because it is the more specific instruction.
	Prefix string   `json:"prefix"`
	Count  int      `json:"count"`
	Start  int      `json:"start"`
	Names  []string `json:"names"`

	// The rest is applied to every user created, exactly like the single form.
	Status            domain.UserStatus           `json:"status"`
	TrafficLimitBytes int64                       `json:"trafficLimitBytes"`
	TrafficReset      domain.TrafficResetStrategy `json:"trafficLimitStrategy"`
	ExpireAt          *time.Time                  `json:"expireAt"`
	Tag               string                      `json:"tag"`
	HWIDDeviceLimit   int                         `json:"hwidDeviceLimit"`
	SquadIDs          []string                    `json:"squadUuids"`
}

// maxBulkCreate bounds one request. It is not a policy about how many users a
// deployment may have — it is a guard against a typo in `count` turning into a
// hundred thousand rows and a sync storm.
const maxBulkCreate = 500

type bulkCreateResult struct {
	Created []userView `json:"created"`
	// Failed names and why, so a partial run is diagnosable rather than
	// mysterious. A name that already exists is the common case.
	Failed []bulkFailure `json:"failed"`
}

type bulkFailure struct {
	Username string `json:"username"`
	Error    string `json:"error"`
}

func (a *API) bulkCreateUsers(w http.ResponseWriter, r *http.Request) {
	var req bulkCreateRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	names, err := req.usernames()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	if !req.Status.Valid() {
		req.Status = domain.UserActive
	}
	if !req.TrafficReset.Valid() {
		req.TrafficReset = domain.ResetNever
	}

	result := bulkCreateResult{Created: []userView{}, Failed: []bulkFailure{}}
	for _, name := range names {
		single := userRequest{
			Username:          name,
			Status:            req.Status,
			TrafficLimitBytes: req.TrafficLimitBytes,
			TrafficReset:      req.TrafficReset,
			ExpireAt:          req.ExpireAt,
			Tag:               req.Tag,
			HWIDDeviceLimit:   req.HWIDDeviceLimit,
			SquadIDs:          req.SquadIDs,
		}
		input, err := single.toInput()
		if err != nil {
			result.Failed = append(result.Failed, bulkFailure{Username: name, Error: err.Error()})
			continue
		}
		// One at a time rather than one transaction: a single duplicate name
		// should cost that name, not the whole batch.
		user, err := a.store.CreateUser(r.Context(), input, newUserSecrets())
		if err != nil {
			result.Failed = append(result.Failed, bulkFailure{Username: name, Error: friendlyStoreErr(err)})
			continue
		}
		result.Created = append(result.Created, a.view(user))
	}

	if len(result.Created) > 0 {
		a.store.LogEvent(r.Context(), domain.EventUserCreated, claimsOf(r).Username, "",
			fmt.Sprintf("%d users created in bulk", len(result.Created)),
			map[string]any{"created": len(result.Created), "failed": len(result.Failed)})
		// One sync for the batch. Syncing per user would push the same node
		// hundreds of times for a single operator action.
		a.hub.RequestSyncAll(r.Context())
	}
	writeJSON(w, http.StatusOK, result)
}

// friendlyStoreErr turns a storage failure into something an operator can act
// on. The common one by far is a name already in use, and saying so beats
// "internal error" beside a row they can fix themselves.
func friendlyStoreErr(err error) string {
	switch {
	case errors.Is(err, postgres.ErrConflict):
		return "a user with this name already exists"
	case errors.Is(err, postgres.ErrNotFound):
		return "not found"
	default:
		return "could not be created"
	}
}

// usernames resolves the request into the exact list to create, so the handler
// never has to reason about which of the two forms was used.
func (req bulkCreateRequest) usernames() ([]string, error) {
	if len(req.Names) > 0 {
		out := make([]string, 0, len(req.Names))
		seen := map[string]bool{}
		for _, n := range req.Names {
			n = strings.TrimSpace(n)
			if n == "" {
				continue
			}
			// A pasted list often contains the same name twice; creating the
			// first and reporting the second as a duplicate is just noise.
			if seen[strings.ToLower(n)] {
				continue
			}
			seen[strings.ToLower(n)] = true
			out = append(out, n)
		}
		if len(out) == 0 {
			return nil, errBadRequest("the list contains no usable names")
		}
		if len(out) > maxBulkCreate {
			return nil, errBadRequest(fmt.Sprintf("at most %d users at a time", maxBulkCreate))
		}
		return out, nil
	}

	prefix := strings.TrimSpace(req.Prefix)
	if prefix == "" {
		return nil, errBadRequest("give either a list of names or a prefix and a count")
	}
	if req.Count < 1 {
		return nil, errBadRequest("count must be at least 1")
	}
	if req.Count > maxBulkCreate {
		return nil, errBadRequest(fmt.Sprintf("at most %d users at a time", maxBulkCreate))
	}
	start := req.Start
	if start < 1 {
		start = 1
	}
	// Fixed width so the names sort the way they read: user-001 before user-010.
	width := len(strconv.Itoa(start + req.Count - 1))
	out := make([]string, 0, req.Count)
	for i := 0; i < req.Count; i++ {
		out = append(out, fmt.Sprintf("%s%0*d", prefix, width, start+i))
	}
	return out, nil
}

// exportUsersCSV hands the operator the list in the form every billing system,
// spreadsheet and mail merge already understands.
func (a *API) exportUsersCSV(w http.ResponseWriter, r *http.Request) {
	users, _, err := a.store.ListUsers(r.Context(), postgres.UserFilter{
		Search: strings.TrimSpace(r.URL.Query().Get("search")),
		Status: strings.TrimSpace(r.URL.Query().Get("status")),
		Tag:    strings.TrimSpace(r.URL.Query().Get("tag")),
		Limit:  10000,
		SortBy: "username",
	})
	if err != nil {
		a.storeErr(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		`attachment; filename="amneziax-users-`+time.Now().UTC().Format("2006-01-02")+`.csv"`)

	c := csv.NewWriter(w)
	defer c.Flush()

	// The subscription link is in here, which is a credential. That is the
	// point of the export — it is what gets handed to subscribers — and it is
	// why this is not something a read-only account can pull.
	_ = c.Write([]string{
		"username", "status", "tag", "used_bytes", "limit_bytes",
		"expire_at", "created_at", "subscription_url",
	})
	for i := range users {
		u := &users[i]
		expire := ""
		if u.ExpireAt != nil {
			expire = u.ExpireAt.UTC().Format(time.RFC3339)
		}
		_ = c.Write([]string{
			u.Username,
			string(u.Status),
			u.Tag,
			strconv.FormatInt(u.UsedTrafficBytes, 10),
			strconv.FormatInt(u.TrafficLimitBytes, 10),
			expire,
			u.CreatedAt.UTC().Format(time.RFC3339),
			a.subURL(u),
		})
	}
}
