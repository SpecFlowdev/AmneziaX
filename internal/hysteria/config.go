// Package hysteria renders hysteria2 server configurations.
//
// It exists beside internal/xray rather than inside it because the two cores
// share nothing: different document shape, different user model, different
// statistics endpoint. The one thing they have in common is that a profile is
// stored as a JSON document — which works here because hysteria reads YAML, and
// JSON is valid YAML.
package hysteria

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNoListen  = errors.New("a hysteria2 config needs a listen address")
	ErrBadConfig = errors.New("invalid hysteria2 config")
)

// User is one subscriber as hysteria2 understands them: a name and a password.
// There is no per-protocol identity the way xray has uuids and passwords side
// by side — authentication is userpass and nothing else.
type User struct {
	// Name is the key in auth.userpass and the key traffic is reported under.
	Name     string
	Password string
}

// AuthKey builds the identifier a user is known by on a hysteria2 node.
//
// Not `<uuid>.<username>`, which is what xray uses: hysteria parses its config
// with a decoder that treats a dot as nesting, so "abc.alice" arrives as a map
// rather than a key and the server refuses to start. Verified against the real
// binary — this is the reason for the underscore, not a preference.
func AuthKey(userID, username string) string {
	return strings.ReplaceAll(userID, ".", "-") + "_" + sanitise(username)
}

// UserIDFromAuthKey recovers the uuid a traffic figure belongs to.
func UserIDFromAuthKey(key string) (string, bool) {
	idx := strings.Index(key, "_")
	if idx <= 0 {
		return "", false
	}
	id := key[:idx]
	if len(id) != 36 {
		return "", false
	}
	return id, true
}

func sanitise(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// RenderOptions is what the panel knows and the profile document does not.
type RenderOptions struct {
	Users []User
	// StatsListen is where the agent reads per-user counters from. It is bound
	// to loopback so the figures are not reachable from outside the node.
	StatsListen string
}

// Render fills a profile document with the users it should serve and returns
// the exact bytes a node runs, plus their hash.
//
// The operator's document is authoritative for everything else — listen
// address, TLS, obfuscation, masquerade — exactly as an xray profile is. Only
// the parts the panel owns are rewritten.
func Render(profile json.RawMessage, opts RenderOptions) ([]byte, string, error) {
	doc := map[string]any{}
	if len(profile) > 0 {
		if err := json.Unmarshal(profile, &doc); err != nil {
			return nil, "", fmt.Errorf("%w: %v", ErrBadConfig, err)
		}
	}
	if _, ok := doc["listen"]; !ok {
		return nil, "", ErrNoListen
	}

	// A map with one entry per user. Rebuilt from scratch rather than merged:
	// a stale entry here is a subscriber who was removed from the panel and can
	// still connect, which is the one failure this whole path exists to avoid.
	userpass := make(map[string]string, len(opts.Users))
	for _, u := range opts.Users {
		if u.Name == "" || u.Password == "" {
			continue
		}
		userpass[u.Name] = u.Password
	}
	doc["auth"] = map[string]any{"type": "userpass", "userpass": userpass}

	listen := opts.StatsListen
	if listen == "" {
		listen = "127.0.0.1:19999"
	}
	doc["trafficStats"] = map[string]any{"listen": listen}

	// Sorted keys, so the same inputs always produce the same bytes — the hash
	// is what decides whether a node restarts, and a map iterated in random
	// order would restart every node on every sync.
	out, err := marshalStable(doc)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(out)
	return out, hex.EncodeToString(sum[:]), nil
}

// marshalStable is encoding/json with indentation, which already sorts map keys.
// The helper exists to make that guarantee explicit at the call site rather than
// a property someone has to remember.
func marshalStable(doc map[string]any) ([]byte, error) {
	return json.MarshalIndent(doc, "", "  ")
}

// Validate checks a document the operator typed, before it can reach a node.
func Validate(profile json.RawMessage) error {
	doc := map[string]any{}
	if err := json.Unmarshal(profile, &doc); err != nil {
		return fmt.Errorf("%w: %v", ErrBadConfig, err)
	}
	if _, ok := doc["listen"]; !ok {
		return ErrNoListen
	}
	if _, ok := doc["tls"]; !ok {
		if _, acme := doc["acme"]; !acme {
			return fmt.Errorf("%w: needs either tls or acme, or the server will not start",
				ErrBadConfig)
		}
	}
	return nil
}

// Starter is a working document an operator can edit rather than write from
// nothing, in the same spirit as the VLESS + REALITY starter profile.
func Starter(domain string) json.RawMessage {
	if domain == "" {
		domain = "example.com"
	}
	return json.RawMessage(fmt.Sprintf(`{
  "listen": ":8443",
  "acme": {
    "domains": [%q],
    "email": "admin@%s"
  },
  "masquerade": {
    "type": "proxy",
    "proxy": {
      "url": "https://news.ycombinator.com/",
      "rewriteHost": true
    }
  }
}`, domain, domain))
}
