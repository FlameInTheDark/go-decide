package openrouter

import (
	"context"
	"errors"
	"strings"
	"testing"

	decide "github.com/FlameInTheDark/go-decide"
)

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   error
		retry  bool
	}{
		{"invalid", 400, `{"error":{"code":400,"message":"Invalid request parameters"}}`, decide.ErrInvalidRequest, false},
		{"unauthorized", 401, `{"error":{"code":401,"message":"Missing Authentication header"}}`, decide.ErrAuth, false},
		{"credits", 402, `{"error":{"code":402,"message":"Insufficient credits."}}`, decide.ErrPayment, false},
		{"forbidden", 403, `{"error":{"code":403,"message":"Only management keys can perform this operation"}}`, decide.ErrForbidden, false},
		{"not found", 404, `{"error":{"code":404,"message":"Resource not found"}}`, decide.ErrNotFound, false},
		{"too large", 413, `{"error":{"code":413,"message":"Request payload too large"}}`, decide.ErrPayloadTooLarge, false},
		{"rate limited", 429, `{"error":{"code":429,"message":"Rate limit exceeded"}}`, decide.ErrRateLimited, true},
		{"server", 500, `{"error":{"code":500,"message":"Internal Server Error"}}`, decide.ErrServer, true},
		{"provider error", 502, `{"error":{"code":502,"message":"Provider returned error"}}`, decide.ErrUnavailable, true},
		{"unavailable", 503, `{"error":{"code":503,"message":"Service temporarily unavailable"}}`, decide.ErrUnavailable, true},
		{"timeout", 524, `{"error":{"code":524,"message":"Request timed out. Please try again later."}}`, decide.ErrTimeout, true},
		{"overloaded", 529, `{"error":{"code":529,"message":"Provider returned error"}}`, decide.ErrUnavailable, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider, _ := newServer(t, tc.status, tc.body)

			_, err := provider.Decide(context.Background(), decide.Request{
				Model:     "typesafe/jev-1.13",
				State:     decide.Text("something happened"),
				Questions: []decide.Question{decide.Noul("ok", "Is everything fine?")},
			})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}

			var decideErr *decide.Error
			if !errors.As(err, &decideErr) {
				t.Fatalf("error is not *decide.Error: %v", err)
			}
			if decideErr.Provider != Name {
				t.Errorf("provider = %q", decideErr.Provider)
			}
			if decideErr.Retryable() != tc.retry {
				t.Errorf("retryable = %v, want %v", decideErr.Retryable(), tc.retry)
			}
		})
	}
}

func TestRequiresAPIKey(t *testing.T) {
	provider := New(WithBaseURL("http://127.0.0.1:1"))

	_, err := provider.Decide(context.Background(), decide.Request{
		State:     decide.Text("hello"),
		Questions: []decide.Question{decide.Noul("ok", "Fine?")},
	})
	if !errors.Is(err, decide.ErrAuth) {
		t.Fatalf("error = %v, want ErrAuth", err)
	}
}

func TestRejectsImages(t *testing.T) {
	provider, _ := newServer(t, 200, decisionsResponse)

	_, err := provider.Decide(context.Background(), decide.Request{
		State:     decide.Text("hello"),
		Images:    []decide.Image{{Base64: "aGVsbG8="}},
		Questions: []decide.Question{decide.Noul("ok", "Fine?")},
	})
	if !errors.Is(err, decide.ErrUnsupported) {
		t.Fatalf("error = %v, want ErrUnsupported", err)
	}
}

func TestRejectsOversizedBody(t *testing.T) {
	provider, _ := newServer(t, 200, decisionsResponse)

	_, err := provider.Decide(context.Background(), decide.Request{
		Model:     "typesafe/jev-1.13",
		State:     decide.Text(strings.Repeat("x", MaxBodyBytes+1)),
		Questions: []decide.Question{decide.Noul("ok", "Fine?")},
	})
	if !errors.Is(err, decide.ErrPayloadTooLarge) {
		t.Fatalf("error = %v, want ErrPayloadTooLarge", err)
	}
}

func TestRejectsUnknownAnswerType(t *testing.T) {
	provider, _ := newServer(t, 200, `{"answers":{"x":{"type":"mystery"}}}`)

	_, err := provider.Decide(context.Background(), decide.Request{
		Model:     "typesafe/jev-1.13",
		State:     decide.Text("hello"),
		Questions: []decide.Question{decide.Noul("x", "Is it?")},
	})
	if !errors.Is(err, decide.ErrDecode) {
		t.Fatalf("error = %v, want ErrDecode", err)
	}
}

func TestCapabilities(t *testing.T) {
	if caps := New().Capabilities(); caps.Images {
		t.Error("the OpenRouter decisions endpoint does not accept images")
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
