package nodeagent

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	nodev1 "github.com/SpecFlowdev/AmneziaX/gen/go/node/v1"
)

func testAgent(hysteriaBinary string) *Agent {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Agent{
		log:      log,
		hysteria: NewHysteria(hysteriaBinary, "/tmp/amneziax-test-agent", log),
	}
}

func TestExtraCoresSkipsXrayBecauseItIsAlreadyApplied(t *testing.T) {
	a := testAgent("")
	err := a.applyExtraCores(context.Background(), &nodev1.ApplyConfig{
		Cores: []*nodev1.CoreConfig{
			{Kind: "xray", Config: []byte(`{}`)},
			{Kind: "", Config: []byte(`{}`)},
		},
	})
	// Applying xray a second time would restart the process for nothing, and
	// the empty kind means the same thing.
	if err != nil {
		t.Fatalf("xray cores should be ignored here, got %v", err)
	}
}

func TestExtraCoresRefusesAnUnknownKindWithoutGuessing(t *testing.T) {
	a := testAgent("")
	err := a.applyExtraCores(context.Background(), &nodev1.ApplyConfig{
		Cores: []*nodev1.CoreConfig{{Kind: "somethingelse", Config: []byte(`{}`)}},
	})
	// Unknown kinds are logged and skipped, never handed to a binary picked by
	// guesswork — but they must not fail the whole push either.
	if err != nil {
		t.Fatalf("an unknown kind should be skipped, not fatal: %v", err)
	}
}

func TestExtraCoresSaysSoWhenHysteriaIsNotInstalled(t *testing.T) {
	a := testAgent("/nonexistent/hysteria")
	err := a.applyExtraCores(context.Background(), &nodev1.ApplyConfig{
		Cores: []*nodev1.CoreConfig{{Kind: "hysteria2", Config: []byte(`{"listen":":8443"}`)}},
	})
	if err == nil {
		t.Fatal("a node without the binary must report that, not silently do nothing")
	}
	// The message has to tell the operator what to do about it.
	if !strings.Contains(err.Error(), "installer") {
		t.Fatalf("the error does not say how to fix it: %v", err)
	}
}

func TestHysteriaAvailableIsFalseWithoutABinary(t *testing.T) {
	if NewHysteria("", "/tmp", slog.Default()).Available() {
		t.Fatal("reported available with no binary configured")
	}
	if NewHysteria("/nonexistent/hysteria", "/tmp", slog.Default()).Available() {
		t.Fatal("reported available for a path that does not exist")
	}
}
