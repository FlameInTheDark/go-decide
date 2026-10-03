package openrouter

import (
	"context"
	"encoding/json"
	"testing"

	decide "github.com/FlameInTheDark/go-decide"
)

func TestDecide(t *testing.T) {
	provider, got := newServer(t, 200, decisionsResponse)

	result, err := provider.Decide(context.Background(), decisionsRequest())
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	if got.path != DecisionsPath {
		t.Errorf("path = %q, want %q", got.path, DecisionsPath)
	}
	if got.method != "POST" {
		t.Errorf("method = %q, want POST", got.method)
	}
	if auth := got.header.Get("Authorization"); auth != "Bearer test-key" {
		t.Errorf("Authorization = %q", auth)
	}

	// Metadata from the response.
	if result.ID != "gen-dec-1789738314-X5e5eKGQdvR9rblyX250" {
		t.Errorf("id = %q", result.ID)
	}
	if result.Upstream != "TypeSafe" {
		t.Errorf("upstream = %q, want TypeSafe", result.Upstream)
	}
	if result.UpstreamModel != "typesafe/jev-1.13-20260917" {
		t.Errorf("upstream model = %q", result.UpstreamModel)
	}
	if result.Model != "typesafe/jev-1.13" {
		t.Errorf("model = %q, want the requested model", result.Model)
	}
	if result.Usage.Cost != 0.000019992 {
		t.Errorf("cost = %v", result.Usage.Cost)
	}
	if result.Usage.InputTokens != 476 || result.Usage.OutputTokens != 70 {
		t.Errorf("usage = %+v", result.Usage)
	}

	// Typed answers.
	isBug, err := result.Noul("is_bug")
	if err != nil {
		t.Fatalf("Noul: %v", err)
	}
	if !isBug.True() || !closeEnough(isBug.Probability, 0.96) {
		t.Errorf("is_bug = %+v", isBug)
	}

	team, err := result.Choice("team")
	if err != nil {
		t.Fatalf("Choice: %v", err)
	}
	if team.Key != "payments" {
		t.Errorf("team = %q, want payments", team.Key)
	}
	if !closeEnough(team.Probability("frontend"), 0.16) {
		t.Errorf("frontend probability = %v", team.Probability("frontend"))
	}

	urgency, err := result.Score("urgency")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if !closeEnough(urgency.Score, 1.99) {
		t.Errorf("score = %v, want 1.99", urgency.Score)
	}
	if urgency.Description(2) != "Blocking revenue right now" {
		t.Errorf("description = %q", urgency.Description(2))
	}
}

func TestRequestPayload(t *testing.T) {
	provider, got := newServer(t, 200, decisionsResponse)

	req := decisionsRequest()
	req.SessionID = "session-1234"
	req.User = "user-7"
	req.Trace = map[string]string{"trace_id": "trace-abc123"}
	req.Extra = map[string]json.RawMessage{
		"provider": json.RawMessage(`{"allow_fallbacks":true}`),
	}

	if _, err := provider.Decide(context.Background(), req); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	if got.body["model"] != "typesafe/jev-1.13" {
		t.Errorf("model = %v", got.body["model"])
	}
	if got.body["session_id"] != "session-1234" {
		t.Errorf("session_id = %v", got.body["session_id"])
	}
	if got.body["user"] != "user-7" {
		t.Errorf("user = %v", got.body["user"])
	}

	trace, ok := got.body["trace"].(map[string]any)
	if !ok || trace["trace_id"] != "trace-abc123" {
		t.Errorf("trace = %v", got.body["trace"])
	}

	routing, ok := got.body["provider"].(map[string]any)
	if !ok || routing["allow_fallbacks"] != true {
		t.Errorf("provider routing = %v", got.body["provider"])
	}

	// An object state must be sent as a JSON object.
	state, ok := got.body["state"].(map[string]any)
	if !ok || state["customer_tier"] != "enterprise" {
		t.Errorf("state = %v", got.body["state"])
	}

	questions := got.body["questions"].(map[string]any)

	// Choice criteria are an object keyed by option.
	team := questions["team"].(map[string]any)
	teamCriteria, ok := team["criteria"].(map[string]any)
	if !ok || teamCriteria["payments"] != "Checkout, billing, or payment processing issues." {
		t.Errorf("team criteria = %v", team["criteria"])
	}

	// Score criteria are an ordered array.
	urgency := questions["urgency"].(map[string]any)
	levels, ok := urgency["criteria"].([]any)
	if !ok || len(levels) != 3 || levels[0] != "Can wait for the next release" {
		t.Errorf("urgency criteria = %v", urgency["criteria"])
	}

	// Noul criteria are an object with exactly the false and true keys.
	isBug := questions["is_bug"].(map[string]any)
	isBugCriteria, ok := isBug["criteria"].(map[string]any)
	if !ok || len(isBugCriteria) != 2 {
		t.Fatalf("is_bug criteria = %v", isBug["criteria"])
	}
	if isBugCriteria["false"] == nil || isBugCriteria["true"] == nil {
		t.Errorf("is_bug criteria must have false and true keys: %v", isBugCriteria)
	}
}

func TestOptionalFieldsOmitted(t *testing.T) {
	provider, got := newServer(t, 200, decisionsResponse)

	req := decide.Request{
		Model: "typesafe/jev-1.13",
		State: decide.Text("something happened"),
		Questions: []decide.Question{
			decide.Noul("ok", "Is everything fine?"),
		},
	}

	if _, err := provider.Decide(context.Background(), req); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	for _, key := range []string{"session_id", "user", "trace", "provider", "images", "keep_alive"} {
		if _, present := got.body[key]; present {
			t.Errorf("%q should be omitted when unset", key)
		}
	}
}

func TestHeaders(t *testing.T) {
	_, got := newServer(t, 200, decisionsResponse)

	provider := New(
		WithBaseURL(got.baseURL),
		WithAPIKey("secret"),
		WithReferer("https://example.com"),
		WithTitle("My App"),
	)

	if _, err := provider.Decide(context.Background(), decide.Request{
		State:     decide.Text("hello"),
		Questions: []decide.Question{decide.Noul("ok", "Fine?")},
	}); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	if auth := got.header.Get("Authorization"); auth != "Bearer secret" {
		t.Errorf("Authorization = %q", auth)
	}
	if referer := got.header.Get("HTTP-Referer"); referer != "https://example.com" {
		t.Errorf("HTTP-Referer = %q", referer)
	}
	if title := got.header.Get("X-OpenRouter-Title"); title != "My App" {
		t.Errorf("X-OpenRouter-Title = %q", title)
	}
}
