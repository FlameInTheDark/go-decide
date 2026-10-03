package ollama

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
	path   string
	method string
	body   map[string]any
	header http.Header
}

// newServer starts a fake Ollama that answers with the given body and records
// the last request.
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

	return New(WithBaseURL(server.URL), WithHTTPClient(server.Client())), got
}

// choiceResponse mirrors the example from the Ollama decision guide.
const choiceResponse = `{
  "model": "nimble",
  "answers": {
    "label": {
      "type": "choice",
      "choice": "bug",
      "probabilities": {"billing": 0.0125, "bug": 0.9781, "account": 0.0093},
      "confidence": 0.8906
    }
  },
  "usage": {"input_tokens": 174, "output_tokens": 1}
}`

// choiceRequest is the matching request from the same guide.
func choiceRequest() decide.Request {
	return decide.Request{
		Model: "nimble",
		State: decide.Text("Our checkout has returned 500 errors since 9am."),
		Questions: []decide.Question{
			decide.Choice("label", "Which label fits this ticket?", decide.Options{
				"billing": "Payments and refunds",
				"bug":     "Software errors",
				"account": "Login and account access",
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
