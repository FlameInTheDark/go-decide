package openrouter

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	decide "github.com/FlameInTheDark/go-decide"
)

// captured holds the request the fake server received.
type captured struct {
	path    string
	method  string
	body    map[string]any
	header  http.Header
	baseURL string
}

// newServer starts a fake OpenRouter that answers with the given status and
// body and records the last request.
func newServer(t *testing.T, status int, body string) (*Provider, *captured) {
	t.Helper()

	got := &captured{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		got.method = r.Method
		got.header = r.Header.Clone()

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &got.body); err != nil {
				t.Errorf("decode request body: %v", err)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	got.baseURL = server.URL

	return New(
		WithBaseURL(server.URL),
		WithAPIKey("test-key"),
		WithHTTPClient(server.Client()),
	), got
}

// decisionsResponse mirrors the example from the OpenRouter API reference.
const decisionsResponse = `{
  "answers": {
    "is_bug": {"noul": 0.96, "type": "noul"},
    "team": {"choice": "payments", "confidence": 0.75, "probabilities": {"account": 0, "frontend": 0.16, "payments": 0.84}, "type": "choice"},
    "urgency": {"confidence": 0.99, "legend": {"0": "Can wait for the next release", "1": "Should be fixed this week", "2": "Blocking revenue right now"}, "probabilities": {"0": 0, "1": 0.01, "2": 0.99}, "score": 1.99, "type": "score"}
  },
  "id": "gen-dec-1789738314-X5e5eKGQdvR9rblyX250",
  "model": "typesafe/jev-1.13-20260917",
  "provider": "TypeSafe",
  "usage": {"cost": 0.000019992, "input_tokens": 476, "output_tokens": 70}
}`

// decisionsRequest is the matching request from the reference.
func decisionsRequest() decide.Request {
	return decide.Request{
		Model: "typesafe/jev-1.13",
		State: decide.Object(map[string]any{
			"customer_tier": "enterprise",
			"ticket":        "My checkout page shows a blank screen after I click Pay. I have tried two browsers.",
		}),
		Questions: []decide.Question{
			decide.Noul("is_bug", "Is the customer reporting a software defect?", decide.NoulCriteria{
				False: "The customer is asking a question or requesting a feature.",
				True:  "The customer describes broken or unexpected product behavior.",
			}),
			decide.Choice("team", "Which team should own this ticket?", decide.Options{
				"account":  "Login, permissions, or profile issues.",
				"frontend": "Rendering, layout, or browser compatibility issues.",
				"payments": "Checkout, billing, or payment processing issues.",
			}),
			decide.Score("urgency", "How urgent is this ticket?", decide.Scale{
				"Can wait for the next release",
				"Should be fixed this week",
				"Blocking revenue right now",
			}),
		},
	}
}

func closeEnough(got, want float64) bool {
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	return diff < 0.0001
}
