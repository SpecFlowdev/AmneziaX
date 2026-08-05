package hysteria

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestAuthKeyAvoidsTheDotHysteriaCannotParse(t *testing.T) {
	// The whole reason this function exists: hysteria's config decoder reads a
	// dot as nesting, so the "<uuid>.<username>" form xray uses arrives as a
	// map and the server refuses to start.
	key := AuthKey("3f1b2c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d", "alice")
	if strings.Contains(key, ".") {
		t.Fatalf("key contains a dot: %s", key)
	}

	id, ok := UserIDFromAuthKey(key)
	if !ok || id != "3f1b2c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d" {
		t.Fatalf("round trip failed: got %q ok=%v", id, ok)
	}
}

func TestAuthKeySanitisesTheUsername(t *testing.T) {
	key := AuthKey("3f1b2c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d", "a.b c/d")
	if strings.ContainsAny(key[37:], ". /") {
		t.Fatalf("username part still has characters that break the parse: %s", key)
	}
	// The uuid must still be recoverable after sanitising.
	if _, ok := UserIDFromAuthKey(key); !ok {
		t.Fatal("uuid no longer recoverable")
	}
}

func TestRenderRewritesUsersRatherThanMerging(t *testing.T) {
	profile := json.RawMessage(`{
		"listen": ":8443",
		"auth": {"type":"userpass","userpass":{"stale_user":"leftover"}},
		"masquerade": {"type":"proxy"}
	}`)

	out, _, err := Render(profile, RenderOptions{Users: []User{{Name: "a_alice", Password: "p1"}}})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	up := doc["auth"].(map[string]any)["userpass"].(map[string]any)
	if _, stale := up["stale_user"]; stale {
		t.Fatal("a removed subscriber survived the render and could still connect")
	}
	if up["a_alice"] != "p1" {
		t.Fatalf("the current user is missing: %v", up)
	}
	// Everything the operator wrote that the panel does not own must survive.
	if _, ok := doc["masquerade"]; !ok {
		t.Fatal("the operator's masquerade block was dropped")
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	profile := json.RawMessage(`{"listen":":8443"}`)
	users := []User{}
	for _, n := range []string{"c_c", "a_a", "b_b", "d_d", "e_e"} {
		users = append(users, User{Name: n, Password: "p"})
	}

	_, first, err := Render(profile, RenderOptions{Users: users})
	if err != nil {
		t.Fatal(err)
	}
	// A hash that changes between identical renders restarts every node on
	// every sync, which is the exact failure this guards.
	for i := 0; i < 20; i++ {
		_, again, err := Render(profile, RenderOptions{Users: users})
		if err != nil {
			t.Fatal(err)
		}
		if again != first {
			t.Fatalf("hash changed between identical renders: %s vs %s", first, again)
		}
	}
}

func TestRenderRefusesADocumentThatCannotListen(t *testing.T) {
	if _, _, err := Render(json.RawMessage(`{"tls":{}}`), RenderOptions{}); err == nil {
		t.Fatal("accepted a config with no listen address")
	}
}

func TestValidateWantsSomethingThatCanServeTLS(t *testing.T) {
	if err := Validate(json.RawMessage(`{"listen":":8443"}`)); err == nil {
		t.Fatal("accepted a config with neither tls nor acme")
	}
	if err := Validate(json.RawMessage(`{"listen":":8443","tls":{"cert":"c","key":"k"}}`)); err != nil {
		t.Fatalf("rejected a valid config: %v", err)
	}
	if err := Validate(json.RawMessage(`{"listen":":8443","acme":{"domains":["x"]}}`)); err != nil {
		t.Fatalf("rejected an acme config: %v", err)
	}
}

// The point of this one is that the real binary starts on what we render.
// Everything above only proves the JSON matches our own expectations.
func TestRenderedConfigIsAcceptedByHysteria(t *testing.T) {
	bin := os.Getenv("HYSTERIA_BIN")
	cert, key := os.Getenv("HYSTERIA_CERT"), os.Getenv("HYSTERIA_KEY")
	if bin == "" || cert == "" || key == "" {
		t.Skip("HYSTERIA_BIN, HYSTERIA_CERT and HYSTERIA_KEY not set")
	}

	profile := json.RawMessage(`{"listen":":18443","tls":{"cert":"` + cert + `","key":"` + key + `"}}`)
	out, _, err := Render(profile, RenderOptions{
		Users: []User{
			{Name: AuthKey("3f1b2c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d", "alice"), Password: "p1"},
			{Name: AuthKey("4f1b2c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d", "bob.name"), Password: "p2"},
		},
		StatsListen: "127.0.0.1:18999",
	})
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.CreateTemp(t.TempDir(), "hy*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(out); err != nil {
		t.Fatal(err)
	}
	f.Close()

	// hysteria has no --test flag, so it is started and given a moment to
	// either come up or refuse the config.
	cmd := exec.Command(bin, "server", "-c", f.Name())
	var buf strings.Builder
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()

	deadline := 6
	for i := 0; i < deadline*10; i++ {
		if strings.Contains(buf.String(), "server up and running") ||
			strings.Contains(buf.String(), "FATAL") {
			break
		}
		exec.Command("sleep", "0.1").Run()
	}
	if !strings.Contains(buf.String(), "server up and running") {
		t.Fatalf("hysteria did not accept the rendered config:\n%s\n---\n%s", buf.String(), out)
	}
}
