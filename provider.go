package decide

import "context"

// Provider is the single method a decision backend has to implement. It is
// the extension point of this package: implement it to add a new backend, then
// hand the value to [Client] with [WithProvider].
//
// Implementations must be safe for concurrent use, must honour ctx
// cancellation and should return a *[Error] (see [NewError] and
// [NewHTTPError]) so callers can classify failures with [errors.Is].
type Provider interface {
	// Name is the stable identifier of the adapter, e.g. "ollama". The
	// [Client] uses it to select a backend.
	Name() string
	// Decide answers the questions in req.
	Decide(ctx context.Context, req Request) (*Result, error)
}

// Closer is implemented by providers that hold resources such as idle
// connections or background goroutines. [Client.Close] calls it when the
// client itself implements it.
type Closer interface {
	Close() error
}

// Capability describes optional provider features. Adapters implement
// [Capable] to advertise them; the [Client] uses the information to fail fast
// instead of letting the provider reject the request.
type Capability struct {
	// Images reports whether the provider accepts images.
	Images bool
	// MaxQuestions is the highest number of questions per request, or 0
	// when unknown.
	MaxQuestions int
	// MaxChoices is the highest number of options or scale levels, or 0
	// when unknown.
	MaxChoices int
	// MaxStateBytes is the largest accepted state, or 0 when unknown.
	MaxStateBytes int
	// MaxImageBytes is the largest accepted image payload, or 0 when unknown.
	MaxImageBytes int
}

// Capable is an optional interface implemented by providers that advertise
// their capabilities.
type Capable interface {
	Capabilities() Capability
}

// ProviderFunc adapts a function to the [Provider] interface, which is handy in
// tests and for one-off backends.
type ProviderFunc struct {
	// ID is the provider name.
	ID string
	// Fn implements the decision call.
	Fn func(ctx context.Context, req Request) (*Result, error)
}

// Name implements [Provider].
func (p ProviderFunc) Name() string { return p.ID }

// Decide implements [Provider].
func (p ProviderFunc) Decide(ctx context.Context, req Request) (*Result, error) {
	return p.Fn(ctx, req)
}
