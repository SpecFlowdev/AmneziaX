package singbox

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const cert = `"tls":{"enabled":true,"certificate_path":"CERT","key_path":"KEY"}`

func doc(inbound string) json.RawMessage {
	return json.RawMessage(`{"log":{"level":"warn"},"inbounds":[` + inbound + `],"outbounds":[{"type":"direct"}]}`)
}

func TestParseInboundsRejectsAmbiguousTags(t *testing.T) {
	_, err := ParseInbounds(doc(`{"type":"tuic","tag":"a","listen_port":1},{"type":"tuic","tag":"a","listen_port":2}`))
	if err == nil {
		// Tags are the identity squads and hosts point at; a duplicate makes
		// one of them silently unreachable.
		t.Fatal("accepted two inbounds sharing a tag")
	}
	if _, err := ParseInbounds(doc(`{"type":"tuic","listen_port":1}`)); err == nil {
		t.Fatal("accepted an inbound with no tag")
	}
	if _, err := ParseInbounds(json.RawMessage(`{"outbounds":[]}`)); err == nil {
		t.Fatal("accepted a config with no inbounds")
	}
}

func TestParseInboundsReadsWhatASquadNeeds(t *testing.T) {
	got, err := ParseInbounds(doc(`{"type":"TUIC","tag":"t","listen_port":8444}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Tag != "t" || got[0].Type != "tuic" || got[0].Port != 8444 {
		t.Fatalf("got %+v", got)
	}
}

func TestRenderReplacesUsersRatherThanMerging(t *testing.T) {
	in := `{"type":"tuic","tag":"t","listen_port":8444,"users":[{"name":"stale","uuid":"x","password":"y"}]}`
	out, _, err := Render(doc(in), RenderOptions{UsersByTag: map[string][]User{
		"t": {{Name: "u1", UUID: "3f1b2c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d", Password: "p"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "stale") {
		t.Fatal("a removed subscriber survived the render and could still connect")
	}
}

func TestRenderSkipsUsersMissingWhatTheTypeNeeds(t *testing.T) {
	// TUIC needs both a uuid and a password. A half-filled entry would be
	// written as a user that can never authenticate, or refused by sing-box.
	out, _, err := Render(doc(`{"type":"tuic","tag":"t","listen_port":1}`), RenderOptions{
		UsersByTag: map[string][]User{"t": {
			{Name: "ok", UUID: "3f1b2c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d", Password: "p"},
			{Name: "nopass", UUID: "4f1b2c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"},
			{Name: "nouuid", Password: "p"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"nopass", "nouuid"} {
		if strings.Contains(string(out), gone) {
			t.Errorf("%s was written despite missing a required field", gone)
		}
	}
	if !strings.Contains(string(out), "ok") {
		t.Error("the complete user was dropped")
	}
}

func TestRenderHonoursActiveTags(t *testing.T) {
	two := `{"type":"tuic","tag":"a","listen_port":1},{"type":"tuic","tag":"b","listen_port":2}`
	out, _, err := Render(doc(two), RenderOptions{ActiveTags: []string{"b"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `"a"`) {
		t.Fatal("an inbound this node does not serve was rendered anyway")
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	users := []User{}
	for _, n := range []string{"e", "b", "d", "a", "c"} {
		users = append(users, User{Name: n, Password: "p"})
	}
	profile := doc(`{"type":"trojan","tag":"t","listen_port":1}`)

	_, first, err := Render(profile, RenderOptions{UsersByTag: map[string][]User{"t": users}})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		_, again, err := Render(profile, RenderOptions{UsersByTag: map[string][]User{"t": users}})
		if err != nil {
			t.Fatal(err)
		}
		if again != first {
			// A hash that wobbles restarts every node on every sync.
			t.Fatalf("hash changed between identical renders: %s vs %s", first, again)
		}
	}
}

// The one that matters: sing-box itself has to accept what we render, for every
// protocol we claim to serve.
func TestRenderedConfigsAreAcceptedBySingBox(t *testing.T) {
	bin, c, k := os.Getenv("SINGBOX_BIN"), os.Getenv("SINGBOX_CERT"), os.Getenv("SINGBOX_KEY")
	if bin == "" || c == "" || k == "" {
		t.Skip("SINGBOX_BIN, SINGBOX_CERT and SINGBOX_KEY not set")
	}
	tls := strings.NewReplacer("CERT", c, "KEY", k).Replace(cert)

	cases := map[string]struct {
		inbound string
		user    User
	}{
		"tuic": {`{"type":"tuic","tag":"t","listen":"::","listen_port":8444,` + tls + `}`,
			User{Name: "u", UUID: "3f1b2c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d", Password: "p"}},
		"hysteria2": {`{"type":"hysteria2","tag":"t","listen":"::","listen_port":8445,` + tls + `}`,
			User{Name: "u", Password: "p"}},
		"vless": {`{"type":"vless","tag":"t","listen":"::","listen_port":8446,` + tls + `}`,
			User{Name: "u", UUID: "3f1b2c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"}},
		"vmess": {`{"type":"vmess","tag":"t","listen":"::","listen_port":8447}`,
			User{Name: "u", UUID: "3f1b2c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"}},
		"trojan": {`{"type":"trojan","tag":"t","listen":"::","listen_port":8448,` + tls + `}`,
			User{Name: "u", Password: "p"}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			out, _, err := Render(doc(tc.inbound), RenderOptions{
				UsersByTag: map[string][]User{"t": {tc.user}},
			})
			if err != nil {
				t.Fatal(err)
			}
			f, err := os.CreateTemp(t.TempDir(), "sb*.json")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.Write(out); err != nil {
				t.Fatal(err)
			}
			f.Close()

			combined, err := exec.Command(bin, "check", "-c", f.Name()).CombinedOutput()
			if err != nil {
				t.Fatalf("sing-box refused the rendered %s config: %v\n%s\n---\n%s",
					name, err, combined, out)
			}
		})
	}
}
