// Package httpx holds the small HTTP helpers shared by the provider adapters.
// It is internal: adapters embed it, applications never import it.
package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultTimeout is used when a [Doer] carries no timeout of its own.
const DefaultTimeout = 60 * time.Second

// Doer performs HTTP requests. *http.Client satisfies it, and so does
// httptest.Server.Client in tests.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// ErrorDecoder turns a non-2xx response into a provider specific error. The
// message is the decoded provider error, and it may be empty when the body is
// not a recognised error shape.
type ErrorDecoder func(provider string, status int, body []byte, header http.Header) error

// Sender posts JSON payloads to a single endpoint and decodes JSON responses.
// It is safe for concurrent use as long as the underlying Doer is.
type Sender struct {
	// Provider is the adapter name used in errors.
	Provider string
	// URL is the absolute endpoint URL.
	URL string
	// Header carries headers applied to every request.
	Header http.Header
	// Doer performs the requests. Nil uses a client with
	// [DefaultTimeout].
	Doer Doer
	// DecodeError maps error responses to provider errors. Nil falls back
	// to a generic message.
	DecodeError ErrorDecoder
	// MaxBodyBytes rejects oversized responses instead of buffering them.
	// Zero means no limit.
	MaxBodyBytes int64
}

// PostJSON encodes payload as JSON, posts it and decodes the response into
// out. A nil out discards the body, which suits "fire and check status" calls.
func (s *Sender) PostJSON(ctx context.Context, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("decide: encode %s request: %w", s.Provider, err)
	}
	return s.do(ctx, http.MethodPost, bytes.NewReader(body), out)
}

// GetJSON performs a GET request and decodes the JSON response into out. A nil
// out discards the body.
func (s *Sender) GetJSON(ctx context.Context, out any) error {
	return s.do(ctx, http.MethodGet, nil, out)
}

// do performs the request and applies the shared status and decoding rules.
func (s *Sender) do(ctx context.Context, method string, body io.Reader, out any) error {
	var reader io.Reader
	if body != nil {
		reader = body
	}

	req, err := http.NewRequestWithContext(ctx, method, s.URL, reader)
	if err != nil {
		return fmt.Errorf("decide: build %s request: %w", s.Provider, err)
	}

	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, values := range s.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	resp, err := s.doer().Do(req)
	if err != nil {
		return fmt.Errorf("decide: %s request failed: %w", s.Provider, err)
	}
	defer resp.Body.Close()

	response, err := readBody(resp.Body, s.MaxBodyBytes)
	if err != nil {
		return fmt.Errorf("decide: read %s response: %w", s.Provider, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return s.decodeError(resp.StatusCode, response, resp.Header)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(response, out); err != nil {
		return fmt.Errorf("decide: decode %s response: %w", s.Provider, err)
	}
	return nil
}

// Doer exposes the Doer in use, creating the default client when unset.
func (s *Sender) doer() Doer {
	if s.Doer != nil {
		return s.Doer
	}
	return &http.Client{Timeout: DefaultTimeout}
}

func (s *Sender) decodeError(status int, body []byte, header http.Header) error {
	if s.DecodeError != nil {
		return s.DecodeError(s.Provider, status, body, header)
	}

	message := strings.TrimSpace(string(body))
	if decoded := decodeStringError(body); decoded != "" {
		message = decoded
	}
	if message == "" {
		message = http.StatusText(status)
	}
	return &HTTPError{Provider: s.Provider, StatusCode: status, Message: message, Body: string(body)}
}

// HTTPError is the fallback error type used when no ErrorDecoder is supplied.
type HTTPError struct {
	Provider   string
	StatusCode int
	Message    string
	Body       string
}

// Error implements the error interface.
func (e *HTTPError) Error() string {
	return fmt.Sprintf("decide %s: HTTP %d: %s", e.Provider, e.StatusCode, e.Message)
}

// decodeStringError recognises the common {"error": "message"} and
// {"error": {"message": "..."}} shapes.
func decodeStringError(body []byte) string {
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || len(envelope.Error) == 0 {
		return ""
	}

	var text string
	if err := json.Unmarshal(envelope.Error, &text); err == nil {
		return text
	}

	var nested struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(envelope.Error, &nested); err == nil && nested.Message != "" {
		return nested.Message
	}
	return ""
}

func readBody(body io.Reader, limit int64) ([]byte, error) {
	if limit > 0 {
		body = io.LimitReader(body, limit)
	}
	return io.ReadAll(body)
}
