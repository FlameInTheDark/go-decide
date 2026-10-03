package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	decide "github.com/FlameInTheDark/go-decide"
)

func TestDecideErrorMapping(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   error
		retry  bool
	}{
		{"not found", 404, `{"error":"model \"nimble\" not found"}`, decide.ErrNotFound, false},
		{"bad request", 400, `{"error":"unsupported model or runner"}`, decide.ErrInvalidRequest, false},
		{"too large", 413, `{"error":"request body must not exceed 64 KiB without images"}`, decide.ErrPayloadTooLarge, false},
		{"rate limited", 429, `{"error":"slow down"}`, decide.ErrRateLimited, true},
		{"server", 500, `{"error":"the model failed to score"}`, decide.ErrServer, true},
		{"bad gateway", 502, `{"error":"cloud model unreachable"}`, decide.ErrUnavailable, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider, _ := newServer(t, tc.status, tc.body)

			_, err := provider.Decide(context.Background(), decide.Request{
				Model:     "nimble",
				State:     decide.Text("checkout is down"),
				Questions: []decide.Question{decide.Noul("broken", "Is it broken?")},
			})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}

			var decideErr *decide.Error
			if !errors.As(err, &decideErr) {
				t.Fatalf("error is not *decide.Error: %v", err)
			}
			if decideErr.Provider != Name {
				t.Errorf("provider = %q, want %q", decideErr.Provider, Name)
			}
			if decideErr.StatusCode != tc.status {
				t.Errorf("status = %d, want %d", decideErr.StatusCode, tc.status)
			}
			if decideErr.Retryable() != tc.retry {
				t.Errorf("retryable = %v, want %v", decideErr.Retryable(), tc.retry)
			}
			if decideErr.Message == "" {
				t.Error("message is empty")
			}
		})
	}
}

func TestDecideValidatesRequest(t *testing.T) {
	tests := []struct {
		name string
		req  decide.Request
	}{
		{
			name: "empty state",
			req: decide.Request{
				Model:     "nimble",
				Questions: []decide.Question{decide.Noul("broken", "Is it broken?")},
			},
		},
		{
			name: "no questions",
			req:  decide.Request{Model: "nimble", State: decide.Text("hello")},
		},
		{
			name: "choice without options",
			req: decide.Request{
				Model:     "nimble",
				State:     decide.Text("hello"),
				Questions: []decide.Question{decide.Choice("label", "Which label?")},
			},
		},
		{
			name: "duplicate question names",
			req: decide.Request{
				Model: "nimble",
				State: decide.Text("hello"),
				Questions: []decide.Question{
					decide.Noul("same", "First?"),
					decide.Noul("same", "Second?"),
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider, _ := newServer(t, 200, choiceResponse)

			_, err := provider.Decide(context.Background(), tc.req)
			if !errors.Is(err, decide.ErrInvalidRequest) {
				t.Fatalf("error = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

func TestDecideRejectsOversizedBody(t *testing.T) {
	provider, _ := newServer(t, 200, choiceResponse)

	_, err := provider.Decide(context.Background(), decide.Request{
		Model: "nimble",
		State: decide.Text(strings.Repeat("x", MaxTextBodyBytes+1)),
		Questions: []decide.Question{
			decide.Noul("broken", "Is it broken?"),
		},
	})
	if !errors.Is(err, decide.ErrPayloadTooLarge) {
		t.Fatalf("error = %v, want ErrPayloadTooLarge", err)
	}
}

func TestDecideUsesDefaultModel(t *testing.T) {
	var model any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make(map[string]any)
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		model = body["model"]
		_, _ = io.WriteString(w, choiceResponse)
	}))
	t.Cleanup(server.Close)

	provider := New(WithBaseURL(server.URL), WithDefaultModel("tev1"))
	req := decide.Request{
		State: decide.Text("hello"),
		Questions: []decide.Question{
			decide.Choice("label", "Which label?", decide.Options{"a": "A", "b": "B"}),
		},
	}

	if _, err := provider.Decide(context.Background(), req); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if model != "tev1" {
		t.Errorf("model = %v, want tev1", model)
	}
}

func TestDecideRejectsUnknownAnswerType(t *testing.T) {
	provider, _ := newServer(t, 200, `{"model":"nimble","answers":{"x":{"type":"mystery"}}}`)

	_, err := provider.Decide(context.Background(), decide.Request{
		Model:     "nimble",
		State:     decide.Text("hello"),
		Questions: []decide.Question{decide.Noul("x", "Is it?")},
	})
	if !errors.Is(err, decide.ErrDecode) {
		t.Fatalf("error = %v, want ErrDecode", err)
	}
}

func TestDecideRequiresAnswers(t *testing.T) {
	provider, _ := newServer(t, 200, `{"model":"nimble","answers":{}}`)

	_, err := provider.Decide(context.Background(), decide.Request{
		Model:     "nimble",
		State:     decide.Text("hello"),
		Questions: []decide.Question{decide.Noul("x", "Is it?")},
	})
	if !errors.Is(err, decide.ErrDecode) {
		t.Fatalf("error = %v, want ErrDecode", err)
	}
}

func TestCapabilities(t *testing.T) {
	if caps := New().Capabilities(); !caps.Images {
		t.Error("Ollama should accept images")
	}
}

func TestProviderRegistered(t *testing.T) {
	for _, name := range decide.Registered() {
		if name == Name {
			return
		}
	}
	t.Errorf("provider %q not registered: %v", Name, decide.Registered())
}
