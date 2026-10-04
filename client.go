package decide

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Factory builds a provider on first use. Adapters register a factory in their
// package init, which lets [Client] resolve a provider by name lazily so that
// importing an adapter never requires configuration such as an API key.
type Factory func() (Provider, error)

var registry = struct {
	mu        sync.RWMutex
	factories map[string]Factory
}{factories: map[string]Factory{}}

// Register makes a provider available to [Client] under the given name. It is
// meant to be called from an adapter's init function. Registering the same
// name twice panics, because that indicates conflicting adapters.
func Register(name string, factory Factory) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	if _, exists := registry.factories[name]; exists {
		panic("decide: provider already registered: " + name)
	}
	registry.factories[name] = factory
}

// Registered lists the names of all registered adapters, sorted alphabetically.
func Registered() []string {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	names := make([]string, 0, len(registry.factories))
	for name := range registry.factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Lookup instantiates the registered adapter with the given name.
func Lookup(name string) (Provider, error) {
	registry.mu.RLock()
	factory, ok := registry.factories[name]
	registry.mu.RUnlock()

	if !ok {
		return nil, &Error{
			Kind:    KindNotFound,
			Message: fmt.Sprintf("decide: no provider registered as %q (registered: %v)", name, Registered()),
		}
	}

	provider, err := factory()
	if err != nil {
		return nil, err
	}
	if provider == nil {
		return nil, &Error{Kind: KindUnknown, Message: "decide: factory for " + name + " returned no provider"}
	}
	if provider.Name() != name {
		return nil, &Error{
			Kind:    KindUnknown,
			Message: fmt.Sprintf("decide: provider reports name %q but is registered as %q", provider.Name(), name),
		}
	}
	return provider, nil
}

// clientState is shared by every copy of a [Client] value.
type clientState struct {
	mu        sync.Mutex
	providers map[string]Provider
	defaultID string
	lazy      bool
	mw        []Middleware
	retry     RetryPolicy
}

// Client routes decision requests to registered providers. It is safe for
// concurrent use.
type Client struct {
	state *clientState
}

// Option configures a [Client].
type Option func(*clientState)

// WithProvider adds a provider instance under its own name, replacing any
// previously configured adapter with that name.
func WithProvider(provider Provider) Option {
	return func(s *clientState) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.providers[provider.Name()] = provider
	}
}

// WithDefault selects the provider used by [Client.Decide].
func WithDefault(name string) Option {
	return func(s *clientState) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.defaultID = name
	}
}

// WithMiddleware appends middleware. Middleware runs in registration order,
// with the first one closest to the caller.
func WithMiddleware(middleware ...Middleware) Option {
	return func(s *clientState) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.mw = append(s.mw, middleware...)
	}
}

// WithRetry sets the retry policy. Pass [NoRetry] to disable retries.
func WithRetry(policy RetryPolicy) Option {
	return func(s *clientState) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.retry = policy
	}
}

// WithLazyProviders enables resolving providers by name through the registry
// instead of requiring [WithProvider]. It is enabled by default so that
// importing an adapter package is enough to use it.
func WithLazyProviders(enabled bool) Option {
	return func(s *clientState) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.lazy = enabled
	}
}

// New builds a client. Without options it resolves providers lazily from the
// registry and performs no retries.
func New(opts ...Option) *Client {
	state := &clientState{
		providers: map[string]Provider{},
		lazy:      true,
		retry:     NoRetry(),
	}
	for _, opt := range opts {
		opt(state)
	}
	return &Client{state: state}
}

// Providers lists the provider names known to the client: explicitly
// configured adapters plus the registered ones when lazy lookup is enabled.
func (c *Client) Providers() []string {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()

	seen := make(map[string]struct{}, len(c.state.providers))
	names := make([]string, 0, len(c.state.providers))
	for name := range c.state.providers {
		seen[name] = struct{}{}
		names = append(names, name)
	}
	if c.state.lazy {
		for _, name := range Registered() {
			if _, ok := seen[name]; !ok {
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names
}

// Provider returns the adapter registered under name, instantiating it through
// the registry when lazy lookup is enabled.
func (c *Client) Provider(name string) (Provider, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	return c.providerLocked(name)
}

func (c *Client) providerLocked(name string) (Provider, error) {
	if provider, ok := c.state.providers[name]; ok {
		return provider, nil
	}
	if !c.state.lazy {
		return nil, &Error{Kind: KindNotFound, Message: "decide: no provider configured as " + name}
	}

	provider, err := Lookup(name)
	if err != nil {
		return nil, err
	}
	c.state.providers[name] = provider
	return provider, nil
}

// Decide sends req to the default provider.
func (c *Client) Decide(ctx context.Context, req Request) (*Result, error) {
	c.state.mu.Lock()
	name := c.state.defaultID
	c.state.mu.Unlock()

	if name == "" {
		return nil, &Error{Kind: KindInvalidRequest, Message: "decide: no default provider, use WithDefault or DecideWith"}
	}
	return c.DecideWith(ctx, name, req)
}

// DecideWith sends req to the named provider. The request is checked against
// the provider's [Capability] before any network call, so an unsupported
// image or an oversized state fails locally.
func (c *Client) DecideWith(ctx context.Context, name string, req Request) (*Result, error) {
	provider, err := c.Provider(name)
	if err != nil {
		return nil, err
	}

	if err := checkCapabilities(provider, req); err != nil {
		return nil, err
	}

	c.state.mu.Lock()
	retry := c.state.retry
	c.state.mu.Unlock()

	ctx = WithProviderContext(ctx, name)

	return retry.Do(ctx, func() (*Result, error) {
		return c.wrap(provider)(ctx, req)
	})
}

// checkCapabilities reports the problems a provider advertises it cannot
// accept, so callers get a precise local error instead of an opaque HTTP
// rejection. A provider that does not implement [Capable] is assumed to accept
// anything and is left to validate the request itself.
func checkCapabilities(provider Provider, req Request) error {
	capable, ok := provider.(Capable)
	if !ok {
		return nil
	}
	cap := capable.Capabilities()

	v := &ValidationError{}
	if len(req.Images) > 0 && !cap.Images {
		v.Add("%s does not accept images, but the request carries %d", provider.Name(), len(req.Images))
	}
	if n := cap.MaxQuestions; n > 0 && len(req.Questions) > n {
		v.Add("%s accepts at most %d questions per request, got %d", provider.Name(), n, len(req.Questions))
	}
	if size, limit := len(req.State.String()), cap.MaxStateBytes; limit > 0 && size > limit {
		v.Add("state is %d bytes, %s accepts at most %d", size, provider.Name(), limit)
	}
	for _, question := range req.Questions {
		if n := len(question.Options); n > cap.MaxChoices && cap.MaxChoices > 0 {
			v.Add("question %q has %d options, %s accepts at most %d",
				question.Name, n, provider.Name(), cap.MaxChoices)
		}
	}

	return v.OrNil()
}

// CheckCapabilities reports whether provider can accept req, according to the
// limits it advertises through [Capable]. [Client.DecideWith] calls it before
// every request; call it directly to check a request up front, for example to
// validate user input before a form is submitted. A nil error means the
// request is within the advertised limits, not that the provider will accept
// it for any other reason.
//
// A provider that does not implement [Capable] is assumed to accept anything,
// which is what [Client.DecideWith] assumes too.
func CheckCapabilities(provider Provider, req Request) error {
	return checkCapabilities(provider, req)
}

// wrap applies the configured middleware chain to a provider. The caller must
// not hold the client mutex.
func (c *Client) wrap(provider Provider) Handler {
	c.state.mu.Lock()
	middleware := append([]Middleware(nil), c.state.mw...)
	c.state.mu.Unlock()

	handler := providerHandler(provider)
	for i := len(middleware) - 1; i >= 0; i-- {
		handler = middleware[i](handler)
	}
	return handler
}

func providerHandler(provider Provider) Handler {
	return func(ctx context.Context, req Request) (*Result, error) {
		return provider.Decide(ctx, req)
	}
}

// Close releases resources held by the client's providers that implement
// [Closer].
func (c *Client) Close() error {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()

	var errs []error
	for _, provider := range c.state.providers {
		closer, ok := provider.(Closer)
		if !ok {
			continue
		}
		if err := closer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
