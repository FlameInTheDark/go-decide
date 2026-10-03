// Package openrouter implements the decide.Provider interface for the
// OpenRouter alpha.decisions API (POST /api/alpha/decisions), which exposes
// hosted System One decision models such as "typesafe/jev-1.13".
//
//	provider := openrouter.New(openrouter.WithAPIKey(key))
//	res, err := provider.Decide(ctx, req)
package openrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	decide "github.com/FlameInTheDark/go-decide"
	"github.com/FlameInTheDark/go-decide/internal/httpx"
)

// Name is the provider identifier used by decide.Register and decide.Client.
const Name = "openrouter"

// DefaultBaseURL is the OpenRouter API root.
const DefaultBaseURL = "https://openrouter.ai"

// DecisionsPath is the alpha decisions endpoint.
const DecisionsPath = "/api/alpha/decisions"

// MaxBodyBytes is the documented request payload limit.
const MaxBodyBytes = 1 << 20

func init() {
	decide.Register(Name, func() (decide.Provider, error) {
		return New(), nil
	})
}

// Provider talks to the OpenRouter decisions API.
type Provider struct {
	apiKey  string
	baseURL string
	referer string
	title   string
	sender  *httpx.Sender
	mu      sync.Mutex
}

// Option configures a [Provider].
type Option func(*Provider)

// WithAPIKey sets the API key. It also falls back to the OPENROUTER_API_KEY
// environment variable when called without an argument value.
func WithAPIKey(key string) Option {
	return func(p *Provider) {
		if key != "" {
			p.apiKey = key
		}
	}
}

// WithBaseURL points the provider at a different API root, which is useful for
// gateways and tests.
func WithBaseURL(baseURL string) Option {
	return func(p *Provider) {
		p.baseURL = strings.TrimRight(baseURL, "/")
	}
}

// WithHTTPClient supplies the HTTP client used for requests.
func WithHTTPClient(client *http.Client) Option {
	return func(p *Provider) {
		p.sender.Doer = client
	}
}

// WithReferer sets the HTTP-Referer header used for app rankings.
func WithReferer(referer string) Option {
	return func(p *Provider) { p.referer = referer }
}

// WithTitle sets the X-OpenRouter-Title header shown in the dashboard.
func WithTitle(title string) Option {
	return func(p *Provider) { p.title = title }
}

// WithHeader adds a header sent with every request.
func WithHeader(key, value string) Option {
	return func(p *Provider) { p.sender.Header.Set(key, value) }
}

// New builds an OpenRouter provider. Without options it reads the API key from
// the OPENROUTER_API_KEY environment variable; the key is only required when a
// request is actually made.
func New(opts ...Option) *Provider {
	provider := &Provider{
		apiKey:  strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")),
		baseURL: DefaultBaseURL,
	}

	provider.sender = &httpx.Sender{
		Provider:     Name,
		URL:          DefaultBaseURL + DecisionsPath,
		Header:       http.Header{},
		MaxBodyBytes: MaxBodyBytes,
		DecodeError:  decodeError,
	}

	for _, opt := range opts {
		opt(provider)
	}

	provider.sender.URL = provider.baseURL + DecisionsPath
	provider.applyHeaders()

	return provider
}

// applyHeaders refreshes the header set from the current configuration.
func (p *Provider) applyHeaders() {
	p.sender.Header.Set("Authorization", "Bearer "+p.apiKey)
	if p.referer != "" {
		p.sender.Header.Set("HTTP-Referer", p.referer)
	}
	if p.title != "" {
		p.sender.Header.Set("X-OpenRouter-Title", p.title)
	}
}

// Name implements [decide.Provider].
func (p *Provider) Name() string { return Name }

// BaseURL returns the configured API root.
func (p *Provider) BaseURL() string { return p.baseURL }

// Capabilities implements [decide.Capable]. The hosted decisions endpoint does
// not accept images.
func (p *Provider) Capabilities() decide.Capability {
	return decide.Capability{
		Images:        false,
		MaxChoices:    decide.MaxCriteria,
		MaxStateBytes: MaxBodyBytes,
	}
}

// Decide implements [decide.Provider].
func (p *Provider) Decide(ctx context.Context, req decide.Request) (*decide.Result, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if len(req.Images) > 0 {
		return nil, decide.NewError(Name, decide.KindUnsupported, fmt.Errorf(
			"the OpenRouter decisions endpoint does not accept images"))
	}

	p.mu.Lock()
	if p.apiKey == "" {
		p.mu.Unlock()
		return nil, decide.NewError(Name, decide.KindAuth, fmt.Errorf(
			"no API key configured, use WithAPIKey or set OPENROUTER_API_KEY"))
	}
	sender := *p.sender
	p.mu.Unlock()

	payload := request{
		SystemOnePayload: req.SystemOnePayload(),
		SessionID:        req.SessionID,
		Trace:            req.Trace,
		User:             req.User,
		Provider:         routing(req.Extra),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, decide.NewError(Name, decide.KindUnknown, fmt.Errorf("encode request: %w", err))
	}
	if len(body) > MaxBodyBytes {
		return nil, decide.NewError(Name, decide.KindPayloadTooLarge, fmt.Errorf(
			"request body is %d bytes, the limit is %d", len(body), MaxBodyBytes))
	}

	var response response
	if err := sender.PostJSON(ctx, payload, &response); err != nil {
		return nil, err
	}

	return response.result(p.Name(), req.Model)
}

// routing pulls the OpenRouter "provider" routing preferences out of the
// request extras, where callers can place them under the "provider" key.
func routing(extra map[string]json.RawMessage) json.RawMessage {
	if extra == nil {
		return nil
	}
	for _, key := range []string{"provider", "provider_preferences"} {
		if raw, ok := extra[key]; ok && len(raw) > 0 && string(raw) != "null" {
			return raw
		}
	}
	return nil
}
