package decide

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Handler is the decision call signature used by middleware.
type Handler func(ctx context.Context, req Request) (*Result, error)

// Middleware wraps a [Handler], which makes it easy to add logging, metrics,
// caching or fallback behaviour to any provider.
//
// The middleware chain is built per provider, so a handler can discover which
// backend it is talking to through [ProviderFromContext] and the helpers below.
type Middleware func(next Handler) Handler

// providerKey is the context key under which the active provider name is
// stored for middleware.
type providerKey struct{}

// ProviderFromContext returns the name of the provider serving the current
// call. It is populated by [Client] before the middleware chain runs.
func ProviderFromContext(ctx context.Context) (string, bool) {
	name, ok := ctx.Value(providerKey{}).(string)
	return name, ok
}

// WithProviderContext returns a context carrying the provider name. Adapters
// that call a provider directly can use it so middleware sees the same value.
func WithProviderContext(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, providerKey{}, name)
}

// WithLogging logs every decision call through the given logger. Pass nil to
// use [slog.Default].
func WithLogging(logger *slog.Logger) Middleware {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next Handler) Handler {
		return func(ctx context.Context, req Request) (*Result, error) {
			start := time.Now()
			result, err := next(ctx, req)
			elapsed := time.Since(start)

			attrs := []slog.Attr{
				slog.String("provider", req.providerName(ctx)),
				slog.String("model", req.Model),
				slog.Int("questions", len(req.Questions)),
				slog.Duration("elapsed", elapsed),
			}

			if err != nil {
				attrs = append(attrs, slog.String("error", err.Error()))
				logger.LogAttrs(ctx, slog.LevelError, "decide request failed", attrs...)
				return result, err
			}

			logger.LogAttrs(ctx, slog.LevelInfo, "decide request", attrs...)
			return result, nil
		}
	}
}

// WithRecover turns a panic inside a provider into an error so a single bad
// adapter cannot take the process down.
func WithRecover() Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req Request) (result *Result, err error) {
			defer func() {
				recovered := recover()
				if recovered == nil {
					return
				}
				result = nil
				err = &Error{
					Provider: req.providerName(ctx),
					Kind:     KindUnknown,
					Message:  "panic in provider: " + describeRecovered(recovered),
				}
			}()
			return next(ctx, req)
		}
	}
}

func describeRecovered(v any) string {
	switch typed := v.(type) {
	case string:
		return typed
	case error:
		return typed.Error()
	default:
		return fmt.Sprintf("%v", typed)
	}
}

// providerName resolves the provider name for logging from the context the
// [Client] populated before the middleware chain ran.
func (r Request) providerName(ctx context.Context) string {
	name, _ := ProviderFromContext(ctx)
	return name
}
