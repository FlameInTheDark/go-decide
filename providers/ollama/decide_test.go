package ollama

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	decide "github.com/FlameInTheDark/go-decide"
)

func TestDecideChoice(t *testing.T) {
	provider, got := newServer(t, 200, choiceResponse)

	result, err := provider.Decide(context.Background(), choiceRequest())
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	if got.path != SystemOnePath {
		t.Errorf("path = %q, want %q", got.path, SystemOnePath)
	}
	if got.method != "POST" {
		t.Errorf("method = %q, want POST", got.method)
	}
	if result.Model != "nimble" {
		t.Errorf("model = %q, want nimble", result.Model)
	}
	// Raw keeps the provider bytes so a caller can inspect anything the typed
	// result does not surface. The UI shows it as the raw provider response.
	if !json.Valid(result.Raw) {
		t.Errorf("Raw is not valid JSON: %q", result.Raw)
	}
	var wire map[string]any
	if err := json.Unmarshal(result.Raw, &wire); err != nil {
		t.Fatalf("unmarshal Raw: %v", err)
	}
	if _, ok := wire["answers"].(map[string]any)["label"]; !ok {
		t.Errorf("Raw does not carry the wire answers: %s", result.Raw)
	}
	if result.Usage.InputTokens != 174 || result.Usage.OutputTokens != 1 {
		t.Errorf("usage = %+v", result.Usage)
	}
	if result.Usage.TotalTokens != 175 {
		t.Errorf("total tokens = %d, want 175", result.Usage.TotalTokens)
	}

	label, err := result.Choice("label")
	if err != nil {
		t.Fatalf("Choice: %v", err)
	}
	if label.Key != "bug" {
		t.Errorf("key = %q, want bug", label.Key)
	}
	if p := label.Probability("bug"); !closeEnough(p, 0.9781) {
		t.Errorf("probability = %v, want 0.9781", p)
	}
	if !closeEnough(label.Confidence, 0.8906) {
		t.Errorf("confidence = %v, want 0.8906", label.Confidence)
	}
	if !closeEnough(label.Margin(), 0.9781-0.0125) {
		t.Errorf("margin = %v", label.Margin())
	}
	if ranked := label.Ranked(); len(ranked) != 3 || ranked[0].Key != "bug" {
		t.Errorf("ranked = %+v", ranked)
	}

	// The wire payload must use the documented question shape.
	questions, ok := got.body["questions"].(map[string]any)
	if !ok {
		t.Fatalf("questions missing from body: %v", got.body)
	}
	labelQ, ok := questions["label"].(map[string]any)
	if !ok {
		t.Fatalf("label question missing: %v", questions)
	}
	if labelQ["type"] != "choice" {
		t.Errorf("type = %v, want choice", labelQ["type"])
	}
	criteria, ok := labelQ["criteria"].(map[string]any)
	if !ok || criteria["bug"] != "Software errors" {
		t.Errorf("criteria = %v", labelQ["criteria"])
	}
	if got.body["state"] != "Our checkout has returned 500 errors since 9am." {
		t.Errorf("state = %v", got.body["state"])
	}
}

func TestDecideScoreAndNoul(t *testing.T) {
	const body = `{
      "model": "nimble",
      "answers": {
        "refund": {"type": "noul", "noul": 0.9989},
        "urgency": {
          "type": "score",
          "score": 0.8308,
          "legend": {"0": "Routine", "1": "Soon", "2": "Immediate"},
          "probabilities": {"0": 0.0846, "1": 0.0846, "2": 0.8308},
          "confidence": 0.72
        }
      },
      "usage": {"input_tokens": 91, "output_tokens": 2}
    }`

	provider, got := newServer(t, 200, body)

	req := decide.Request{
		Model: "nimble",
		State: decide.Object(map[string]any{"ticket": "I was charged twice. Please refund the extra payment."}),
		Questions: []decide.Question{
			decide.Noul("refund", "Is the customer requesting a refund?", decide.NoulCriteria{
				False: "No refund is requested",
				True:  "The customer requests a refund",
			}),
			decide.Score("urgency", "How urgently does this ticket need a response?", decide.Scale{
				"Routine: no time pressure",
				"Soon: a customer is inconvenienced",
				"Immediate: a critical service is unavailable",
			}),
		},
	}

	result, err := provider.Decide(context.Background(), req)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	refund, err := result.Noul("refund")
	if err != nil {
		t.Fatalf("Noul: %v", err)
	}
	if !refund.True() {
		t.Errorf("noul = %v, want true", refund.Probability)
	}

	urgency, err := result.Score("urgency")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if urgency.Max() != 2 {
		t.Errorf("max = %d, want 2", urgency.Max())
	}
	if urgency.Level() != 1 {
		t.Errorf("level = %d, want 1", urgency.Level())
	}
	if urgency.Best() != 2 {
		t.Errorf("best = %d, want 2", urgency.Best())
	}
	if urgency.Description(2) != "Immediate" {
		t.Errorf("description = %q", urgency.Description(2))
	}

	// Score criteria must travel as an array, noul criteria as an object.
	questions := got.body["questions"].(map[string]any)
	score := questions["urgency"].(map[string]any)
	if _, ok := score["criteria"].([]any); !ok {
		t.Errorf("score criteria = %T, want []any", score["criteria"])
	}
	noul := questions["refund"].(map[string]any)
	noulCriteria, ok := noul["criteria"].(map[string]any)
	if !ok || noulCriteria["true"] != "The customer requests a refund" {
		t.Errorf("noul criteria = %v", noul["criteria"])
	}

	// An object state must be sent as a JSON object.
	if _, ok := got.body["state"].(map[string]any); !ok {
		t.Errorf("state = %T, want map", got.body["state"])
	}
}

func TestDecideImages(t *testing.T) {
	const body = `{
      "model": "clef-flash",
      "answers": {"has_ollama": {"type": "noul", "noul": 0.959}},
      "usage": {"input_tokens": 677, "output_tokens": 0}
    }`

	provider, got := newServer(t, 200, body)

	image := decide.Image{Base64: base64.StdEncoding.EncodeToString([]byte("fake png bytes"))}
	req := decide.Request{
		Model:     "clef-flash",
		State:     decide.Text("A user took this screenshot and wants to know what it shows."),
		Images:    []decide.Image{image},
		KeepAlive: 5 * time.Minute,
		Questions: []decide.Question{
			decide.Noul("has_ollama", "Does this image contain Ollama?"),
		},
	}

	if _, err := provider.Decide(context.Background(), req); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	images, ok := got.body["images"].([]any)
	if !ok || len(images) != 1 {
		t.Fatalf("images = %v", got.body["images"])
	}
	if images[0] != image.Base64 {
		t.Errorf("image = %v, want raw base64", images[0])
	}
	if got.body["keep_alive"] != "5m0s" {
		t.Errorf("keep_alive = %v", got.body["keep_alive"])
	}
}
