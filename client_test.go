package decide

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

// stubProvider is a configurable provider for client tests.
type stubProvider struct {
	id      string
	calls   atomic.Int64
	result  *Result
	err     error
	capabil Capability
}

func (s *stubProvider) Name() string { return s.id }

func (s *stubProvider) Decide(_ context.Context, _ Request) (*Result, error) {
	s.calls.Add(1)
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return &Result{Provider: s.id, Answers: NewAnswers(nil)}, nil
}

func (s *stubProvider) Capabilities() Capability { return s.capabil }

func okRequest() Request {
	return Request{
		Model:     "nimble",
		State:     Text("hello"),
		Questions: []Question{Noul("ok", "Fine?")},
	}
}

func TestClientDecideWithDefault(t *testing.T) {
	stub := &stubProvider{id: "stub"}
	client := New(WithProvider(stub), WithDefault("stub"))

	result, err := client.Decide(context.Background(), okRequest())
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if result.Provider != "stub" {
		t.Errorf("provider = %q", result.Provider)
	}
	if stub.calls.Load() != 1 {
		t.Errorf("calls = %d", stub.calls.Load())
	}
}

func TestClientDecideWithoutDefault(t *testing.T) {
	client := New(WithProvider(&stubProvider{id: "stub"}))

	if _, err := client.Decide(context.Background(), okRequest()); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestClientRoutesByName(t *testing.T) {
	first := &stubProvider{id: "first"}
	second := &stubProvider{id: "second"}

	client := New(WithProvider(first), WithProvider(second), WithDefault("first"))

	if _, err := client.DecideWith(context.Background(), "second", okRequest()); err != nil {
		t.Fatalf("DecideWith: %v", err)
	}
	if first.calls.Load() != 0 {
		t.Error("first should not have been called")
	}
	if second.calls.Load() != 1 {
		t.Errorf("second calls = %d", second.calls.Load())
	}
}

func TestClientUnknownProvider(t *testing.T) {
	client := New(WithLazyProviders(false))

	_, err := client.DecideWith(context.Background(), "nope", okRequest())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestClientListsProvidersSorted(t *testing.T) {
	client := New(WithProvider(&stubProvider{id: "beta"}), WithProvider(&stubProvider{id: "alpha"}))

	got := client.Providers()
	if len(got) < 2 {
		t.Fatalf("providers = %v", got)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Errorf("providers are not sorted: %v", got)
			break
		}
	}
}

func TestClientMiddlewareOrder(t *testing.T) {
	var order []string

	record := func(name string) Middleware {
		return func(next Handler) Handler {
			return func(ctx context.Context, req Request) (*Result, error) {
				order = append(order, name)
				return next(ctx, req)
			}
		}
	}

	client := New(
		WithProvider(&stubProvider{id: "stub"}),
		WithDefault("stub"),
		WithMiddleware(record("outer"), record("inner")),
	)

	if _, err := client.Decide(context.Background(), okRequest()); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	if len(order) != 2 || order[0] != "outer" || order[1] != "inner" {
		t.Errorf("order = %v, want [outer inner]", order)
	}
}

func TestMiddlewareSeesProviderName(t *testing.T) {
	var seen string

	capture := func(next Handler) Handler {
		return func(ctx context.Context, req Request) (*Result, error) {
			name, _ := ProviderFromContext(ctx)
			seen = name
			return next(ctx, req)
		}
	}

	client := New(WithProvider(&stubProvider{id: "spy"}), WithDefault("spy"), WithMiddleware(capture))

	if _, err := client.Decide(context.Background(), okRequest()); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if seen != "spy" {
		t.Errorf("provider name = %q, want spy", seen)
	}
}

func TestWithRecoverCatchesPanic(t *testing.T) {
	panicking := ProviderFunc{
		ID: "boom",
		Fn: func(context.Context, Request) (*Result, error) {
			panic("kaboom")
		},
	}

	client := New(WithProvider(panicking), WithDefault("boom"), WithMiddleware(WithRecover()))

	_, err := client.Decide(context.Background(), okRequest())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "kaboom") {
		t.Errorf("error = %v, want it to mention kaboom", err)
	}
}

func TestRegisterAndLookup(t *testing.T) {
	Register("test-stub", func() (Provider, error) {
		return &stubProvider{id: "test-stub"}, nil
	})

	found := false
	for _, name := range Registered() {
		if name == "test-stub" {
			found = true
		}
	}
	if !found {
		t.Fatalf("registered = %v", Registered())
	}

	provider, err := Lookup("test-stub")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if provider.Name() != "test-stub" {
		t.Errorf("name = %q", provider.Name())
	}

	if _, err := Lookup("never-registered"); !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestClientResolvesRegisteredProviderLazily(t *testing.T) {
	Register("lazy-stub", func() (Provider, error) {
		return &stubProvider{id: "lazy-stub"}, nil
	})

	client := New(WithDefault("lazy-stub"))
	if _, err := client.Decide(context.Background(), okRequest()); err != nil {
		t.Fatalf("Decide: %v", err)
	}
}
