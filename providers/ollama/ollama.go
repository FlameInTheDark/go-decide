// Package ollama implements the decide.Provider interface for a local or
// remote Ollama server using the System One endpoint.
//
// Ollama needs no API key. Decision models are available locally from Ollama
// v0.35.0: "nimble" and "tev1" for text, "clef" and "clef-flash" when the
// state includes images.
//
//	provider := ollama.New() // http://localhost:11434
//	res, err := provider.Decide(ctx, req)
package ollama

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
const Name = "ollama"

// DefaultBaseURL is the address of a local Ollama server.
const DefaultBaseURL = "http://localhost:11434"

// DefaultModel is used when a request does not name one.
const DefaultModel = "nimble"

const (
	// SystemOnePath is the decision endpoint. Ollama v0.35.0 or later is
	// required.
	SystemOnePath = "/v1/systemone"
	// TagsPath lists installed models.
	TagsPath = "/api/tags"
	// VersionPath reports the server version.
	VersionPath = "/api/version"
)

// Body size limits enforced by the server, checked locally to give a clearer
// error than an opaque HTTP 413.
const (
	// MaxTextBodyBytes is the limit for requests without images.
	MaxTextBodyBytes = 64 << 10
	// MaxImageBodyBytes is the limit for requests with images.
	MaxImageBodyBytes = 32 << 20
)

func init() {
	decide.Register(Name, func() (decide.Provider, error) {
		return New(), nil
	})
}

// Provider talks to an Ollama server.
type Provider struct {
	baseURL string
	model   string
	sender  *httpx.Sender
	tags    *httpx.Sender
	version *httpx.Sender

	mu sync.Mutex
}

// Option configures a [Provider].
type Option func(*Provider)

// WithBaseURL points the provider at a different server. A trailing slash is
// trimmed.
func WithBaseURL(baseURL string) Option {
	return func(p *Provider) {
		p.baseURL = strings.TrimRight(baseURL, "/")
	}
}

// WithAPIKey authenticates against a hosted Ollama instance. Local servers
// need no key.
func WithAPIKey(key string) Option {
	return func(p *Provider) {
		if key != "" {
			p.sender.Header.Set("Authorization", "Bearer "+key)
		}
	}
}

// WithHTTPClient supplies the HTTP client used for requests.
func WithHTTPClient(client *http.Client) Option {
	return func(p *Provider) {
		p.sender.Doer = client
		p.tags.Doer = client
		p.version.Doer = client
	}
}

// WithDefaultModel sets the model used when a request leaves Model empty.
func WithDefaultModel(model string) Option {
	return func(p *Provider) {
		p.model = model
	}
}

// WithHeader adds a header sent with every request.
func WithHeader(key, value string) Option {
	return func(p *Provider) {
		p.sender.Header.Set(key, value)
		p.tags.Header.Set(key, value)
		p.version.Header.Set(key, value)
	}
}

// New builds an Ollama provider. Without options it targets the local server
// on [DefaultBaseURL] and defaults the model to [DefaultModel]. The address
// can also come from the OLLAMA_HOST environment variable.
func New(opts ...Option) *Provider {
	provider := &Provider{
		baseURL: defaultBaseURL(),
		model:   DefaultModel,
	}

	provider.sender = &httpx.Sender{
		Provider:     Name,
		URL:          provider.baseURL + SystemOnePath,
		Header:       http.Header{},
		MaxBodyBytes: MaxImageBodyBytes,
		DecodeError:  decodeError,
	}
	provider.tags = &httpx.Sender{
		Provider: Name,
		URL:      provider.baseURL + TagsPath,
		Header:   http.Header{},
	}
	provider.version = &httpx.Sender{
		Provider: Name,
		URL:      provider.baseURL + VersionPath,
		Header:   http.Header{},
	}

	for _, opt := range opts {
		opt(provider)
	}

	// Options may have changed the base URL, so recompute the endpoints.
	provider.sender.URL = provider.baseURL + SystemOnePath
	provider.tags.URL = provider.baseURL + TagsPath
	provider.version.URL = provider.baseURL + VersionPath

	return provider
}

func defaultBaseURL() string {
	if host := strings.TrimSpace(os.Getenv("OLLAMA_HOST")); host != "" {
		if !strings.Contains(host, "://") {
			host = "http://" + host
		}
		return strings.TrimRight(host, "/")
	}
	return DefaultBaseURL
}

// Name implements [decide.Provider].
func (p *Provider) Name() string { return Name }

// BaseURL returns the configured server address.
func (p *Provider) BaseURL() string { return p.baseURL }

// Capabilities implements [decide.Capable]. Ollama accepts images with vision
// models such as clef and clef-flash.
func (p *Provider) Capabilities() decide.Capability {
	return decide.Capability{
		Images:        true,
		MaxChoices:    decide.MaxCriteria,
		MaxStateBytes: decide.MaxStateBytes,
		MaxImageBytes: decide.MaxImageBytes,
	}
}

// Decide implements [decide.Provider].
func (p *Provider) Decide(ctx context.Context, req decide.Request) (*decide.Result, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	payload := req.SystemOnePayload()
	if payload.Model == "" {
		payload.Model = p.model
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, decide.NewError(Name, decide.KindUnknown, fmt.Errorf("encode request: %w", err))
	}

	limit := int64(MaxTextBodyBytes)
	if len(req.Images) > 0 {
		limit = MaxImageBodyBytes
	}
	if int64(len(body)) > limit {
		return nil, decide.NewError(Name, decide.KindPayloadTooLarge, fmt.Errorf(
			"request body is %d bytes, the limit is %d", len(body), limit))
	}

	p.mu.Lock()
	sender := *p.sender
	p.mu.Unlock()

	var response response
	raw, err := sender.PostJSON(ctx, payload, &response)
	if err != nil {
		return nil, err
	}

	result, err := response.result(p.Name(), payload.Model)
	if err != nil {
		return nil, err
	}
	result.Raw = json.RawMessage(raw)

	return result, nil
}
