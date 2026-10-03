package ollama

import (
	"context"
	"os"
	"testing"
	"time"

	decide "github.com/FlameInTheDark/go-decide"
)

// TestLiveDecide runs against a real Ollama server. It is skipped unless
// DECIDE_LIVE_OLLAMA=1 is set, so the normal test run stays hermetic:
//
//	DECIDE_LIVE_OLLAMA=1 go test ./providers/ollama -run TestLive -v
func TestLiveDecide(t *testing.T) {
	if os.Getenv("DECIDE_LIVE_OLLAMA") != "1" {
		t.Skip("set DECIDE_LIVE_OLLAMA=1 to run against a local Ollama server")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	provider := New(WithBaseURL(envOr("OLLAMA_BASE_URL", DefaultBaseURL)), WithDefaultModel("nimble"))

	supported, err := provider.SupportsSystemOne(ctx)
	if err != nil {
		t.Fatalf("ServerVersion: %v", err)
	}
	if !supported {
		t.Fatalf("server does not support System One, upgrade Ollama to v0.35.0 or later")
	}

	models, err := provider.DecisionModelNames(ctx)
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(models) == 0 {
		t.Skip("no decision models installed, run: ollama pull nimble")
	}
	t.Logf("decision models: %v", models)

	req := decide.Request{
		State: decide.Text("Our checkout has returned 500 errors since 9am. Customers cannot pay."),
		Questions: []decide.Question{
			decide.Choice("label", "Which label fits this ticket?").
				WithOption("billing", "Payments and refunds").
				WithOption("bug", "Software errors").
				WithOption("account", "Login and account access"),
			decide.Noul("refund", "Is the customer explicitly requesting a refund?"),
			decide.Score("urgency", "How urgent is this ticket?").
				WithScale("Can wait for the next release", "Should be fixed this week", "Blocking revenue right now"),
		},
	}

	result, err := provider.Decide(ctx, req)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	t.Logf("model=%s usage=%+v", result.Model, result.Usage)

	if len(result.Missing(req.Questions)) != 0 {
		t.Errorf("missing answers: %v", result.Missing(req.Questions))
	}

	label, err := result.Choice("label")
	if err != nil {
		t.Fatalf("Choice: %v", err)
	}
	t.Logf("label=%s probability=%.4f confidence=%.4f ranked=%+v",
		label.Key, label.Probability(label.Key), label.Confidence, label.Ranked())

	// The ticket clearly describes a software fault, so "bug" must win.
	if label.Key != "bug" {
		t.Errorf("label = %q, want bug (state describes a software failure)", label.Key)
	}

	refund, err := result.Noul("refund")
	if err != nil {
		t.Fatalf("Noul: %v", err)
	}
	t.Logf("refund probability=%.4f", refund.Probability)
	if p := refund.Probability; p < 0 || p > 1 {
		t.Errorf("noul probability %v is outside [0,1]", p)
	}

	urgency, err := result.Score("urgency")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	t.Logf("urgency score=%.4f level=%d (%s) confidence=%.4f",
		urgency.Score, urgency.Level(), urgency.Description(urgency.Level()), urgency.Confidence)

	if s := urgency.Score; s < 0 || s > 2 {
		t.Errorf("score %v is outside the 0-2 scale", s)
	}
	// Money is blocked, so urgency should lean to the top of the scale.
	if urgency.Score < 1 {
		t.Errorf("urgency score = %v, want at least 1 for a revenue blocking incident", urgency.Score)
	}
}

func TestLiveDecideValidationIsLocal(t *testing.T) {
	if os.Getenv("DECIDE_LIVE_OLLAMA") != "1" {
		t.Skip("set DECIDE_LIVE_OLLAMA=1 to run against a local Ollama server")
	}

	// A request that violates the documented limits must fail locally, without
	// ever reaching the server.
	_, err := New().Decide(context.Background(), decide.Request{
		State:     decide.Text("hello"),
		Questions: []decide.Question{decide.Choice("label", "Which label?")},
	})
	if err == nil {
		t.Fatal("expected a validation error for a choice question without options")
	}
	t.Logf("validation error: %v", err)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
