// Package singbox renders sing-box server configurations.
//
// It earns its place beside xray and hysteria for two reasons. One binary
// serves TUIC, Hysteria2, VLESS, VMess, Trojan and Shadowsocks, so a protocol
// added here costs a renderer rather than another process on every node. And
// unlike hysteria it ships an offline validator — `sing-box check` — which
// means a document can be refused while the operator is still looking at it,
// instead of on a node at restart time.
package singbox

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNoInbounds = errors.New("a sing-box config needs at least one inbound")
	ErrBadConfig  = errors.New("invalid sing-box config")
)

// User is one subscriber. Which fields matter depends on the inbound type, so
// all of them travel and the renderer picks.
type User struct {
	// Name is what sing-box logs and counts traffic against. It carries the
	// uuid so a figure can be charged back even if a username is reused later.
	Name     string
	UUID     string
	Password string
	Flow     string
}

// Inbound is the part of an inbound the panel needs to know about: enough to
// let a squad grant it and a host publish it.
type Inbound struct {
	Tag  string `json:"tag"`
	Type string `json:"type"`
	Port int    `json:"port"`
}

type inboundShape struct {
	Tag        string `json:"tag"`
	Type       string `json:"type"`
	ListenPort int    `json:"listen_port"`
}

// ParseInbounds pulls out the inbounds a squad can grant.
func ParseInbounds(config json.RawMessage) ([]Inbound, error) {
	var doc struct {
		Inbounds []json.RawMessage `json:"inbounds"`
	}
	if err := json.Unmarshal(config, &doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadConfig, err)
	}

	out := make([]Inbound, 0, len(doc.Inbounds))
	seen := map[string]bool{}
	for _, raw := range doc.Inbounds {
		var in inboundShape
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrBadConfig, err)
		}
		if in.Tag == "" {
			return nil, fmt.Errorf("%w: every inbound needs a tag", ErrBadConfig)
		}
		if seen[in.Tag] {
			// Tags are the identity squads and hosts point at, so a duplicate
			// would silently make one of them unreachable.
			return nil, fmt.Errorf("%w: two inbounds share the tag %q", ErrBadConfig, in.Tag)
		}
		seen[in.Tag] = true
		out = append(out, Inbound{Tag: in.Tag, Type: strings.ToLower(in.Type), Port: in.ListenPort})
	}
	if len(out) == 0 {
		return nil, ErrNoInbounds
	}
	return out, nil
}

func Validate(config json.RawMessage) error {
	_, err := ParseInbounds(config)
	return err
}

type RenderOptions struct {
	// UsersByTag is who may use each inbound, keyed by inbound tag.
	UsersByTag map[string][]User
	// ActiveTags limits which inbounds this node serves. Empty means all.
	ActiveTags []string
}

// Render writes the users into each inbound in the shape its type expects, and
// returns the bytes a node runs plus their hash.
func Render(profile json.RawMessage, opts RenderOptions) ([]byte, string, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(profile, &doc); err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrBadConfig, err)
	}
	var inbounds []json.RawMessage
	if raw, ok := doc["inbounds"]; ok {
		if err := json.Unmarshal(raw, &inbounds); err != nil {
			return nil, "", fmt.Errorf("%w: %v", ErrBadConfig, err)
		}
	}

	active := map[string]bool{}
	for _, t := range opts.ActiveTags {
		active[t] = true
	}

	rendered := make([]json.RawMessage, 0, len(inbounds))
	for _, raw := range inbounds {
		var shape inboundShape
		if err := json.Unmarshal(raw, &shape); err != nil {
			return nil, "", fmt.Errorf("%w: %v", ErrBadConfig, err)
		}
		if len(active) > 0 && !active[shape.Tag] {
			continue
		}
		patched, err := injectUsers(raw, strings.ToLower(shape.Type), opts.UsersByTag[shape.Tag])
		if err != nil {
			return nil, "", fmt.Errorf("inbound %q: %w", shape.Tag, err)
		}
		rendered = append(rendered, patched)
	}
	if len(rendered) == 0 {
		return nil, "", ErrNoInbounds
	}

	encoded, err := json.Marshal(rendered)
	if err != nil {
		return nil, "", err
	}
	doc["inbounds"] = encoded

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(out)
	return out, hex.EncodeToString(sum[:]), nil
}

// injectUsers rewrites the users array for one inbound.
//
// The array is replaced rather than merged: an entry left behind here is a
// subscriber who was removed from the panel and can still connect.
func injectUsers(raw json.RawMessage, kind string, users []User) (json.RawMessage, error) {
	var inbound map[string]json.RawMessage
	if err := json.Unmarshal(raw, &inbound); err != nil {
		return nil, err
	}

	entries := make([]map[string]any, 0, len(users))
	for _, u := range users {
		var e map[string]any
		switch kind {
		case "vless":
			if u.UUID == "" {
				continue
			}
			e = map[string]any{"name": u.Name, "uuid": u.UUID}
			if u.Flow != "" {
				e["flow"] = u.Flow
			}
		case "vmess":
			if u.UUID == "" {
				continue
			}
			e = map[string]any{"name": u.Name, "uuid": u.UUID}
		case "tuic":
			// TUIC wants both: the uuid identifies, the password authenticates.
			if u.UUID == "" || u.Password == "" {
				continue
			}
			e = map[string]any{"name": u.Name, "uuid": u.UUID, "password": u.Password}
		case "trojan", "hysteria2", "shadowsocks":
			if u.Password == "" {
				continue
			}
			e = map[string]any{"name": u.Name, "password": u.Password}
		default:
			// An inbound with no per-user identity — a direct or mixed listener,
			// say — is passed through exactly as the operator wrote it.
			return raw, nil
		}
		entries = append(entries, e)
	}

	encoded, err := json.Marshal(entries)
	if err != nil {
		return nil, err
	}
	inbound["users"] = encoded
	return json.Marshal(inbound)
}

// Starter is a working TUIC + Hysteria2 document to edit rather than write from
// nothing, in the same spirit as the VLESS + REALITY starter.
func Starter(domain string) json.RawMessage {
	if domain == "" {
		domain = "example.com"
	}
	return json.RawMessage(fmt.Sprintf(`{
  "log": {"level": "warn"},
  "inbounds": [
    {
      "type": "tuic",
      "tag": "tuic-in",
      "listen": "::",
      "listen_port": 8444,
      "congestion_control": "bbr",
      "tls": {
        "enabled": true,
        "server_name": %q,
        "acme": {"domain": [%q], "email": "admin@%s"}
      }
    },
    {
      "type": "hysteria2",
      "tag": "hysteria2-in",
      "listen": "::",
      "listen_port": 8445,
      "tls": {
        "enabled": true,
        "server_name": %q,
        "acme": {"domain": [%q], "email": "admin@%s"}
      }
    }
  ],
  "outbounds": [{"type": "direct"}]
}`, domain, domain, domain, domain, domain, domain))
}
