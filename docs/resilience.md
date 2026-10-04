# Retries and middleware

Two ways to make a decision pipeline survive a bad day: retry transient
failures, and wrap the call so every provider gets logging, metrics and panic
recovery without repeating yourself.

## Retries

`RetryPolicy` is off by default — a bare `decide.New()` makes one attempt.
Opt in:

```go
client := decide.New(
    decide.WithProvider(ollama.New()),
    decide.WithDefault(ollama.Name),
    decide.WithRetry(decide.DefaultRetry()),
)
```

`DefaultRetry()` is three attempts, exponential backoff from 250 ms up to 5 s,
with 20% jitter.

```go
type RetryPolicy struct {
    MaxAttempts int                    // total attempts, including the first
    BaseDelay   time.Duration          // delay before the second attempt
    MaxDelay    time.Duration          // cap on the backoff
    Jitter      float64                // randomise within ±Jitter (0 disables)
    Rand        *rand.Rand             // source the jitter, for reproducible tests
    RetryOn     func(error) bool       // decide whether an error is worth retrying
}
```

Build your own by starting from `DefaultRetry()` and adjusting, so you keep the
parts you did not mean to change:

```go
policy := decide.DefaultRetry()
policy.MaxAttempts = 5
policy.BaseDelay = 100 * time.Millisecond
policy.MaxDelay = 2 * time.Second

client := decide.New(
    decide.WithProvider(ollama.New()),
    decide.WithDefault(ollama.Name),
    decide.WithRetry(policy),
)
```

Or disable retries entirely:

```go
client := decide.New(decide.WithRetry(decide.NoRetry()))
```

The zero `RetryPolicy` also means one attempt, so `WithRetry(decide.RetryPolicy{})`
and `NoRetry()` are equivalent.

### What gets retried

By default, anything whose error reports itself as retryable — rate limits,
timeouts, 5xx and unreachable hosts. A malformed request or a rejected API key
is **not** retried, because it will fail identically next time.

Override that when your provider has its own idea:

```go
policy := decide.RetryPolicy{
    MaxAttempts: 3,
    BaseDelay:   250 * time.Millisecond,
    MaxDelay:    2 * time.Second,
    Jitter:      0.2,
    RetryOn: func(err error) bool {
        var dErr *decide.Error
        if !errors.As(err, &dErr) {
            return false
        }
        return dErr.Kind == decide.KindRateLimited || dErr.StatusCode == 409
    },
}
```

If the provider sent a `Retry-After`, the library waits for it when it is longer
than the computed backoff, so you do not hammer an endpoint that told you to
slow down.

### Cancellation

Backoff sleeps on the context, so cancelling during a retry wait returns
immediately instead of sitting out the delay. Give the client a deadline and
your retry budget respects it:

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

res, err := client.Decide(ctx, req)
```

### Using a policy directly

`Do` works on any function, which is useful when the thing you want to retry is
not a decision call:

```go
policy := decide.DefaultRetry()

res, err := policy.Do(ctx, func() (*decide.Result, error) {
    return client.Decide(ctx, req)
})
```

## Middleware

A `Middleware` wraps the decision handler, so logging, metrics, caching or
fallbacks apply to every provider at once:

```go
type Middleware func(next Handler) Handler
type Handler func(ctx context.Context, req Request) (*Result, error)
```

Middleware runs in registration order, with the first one closest to the caller.

```go
client := decide.New(
    decide.WithProvider(ollama.New()),
    decide.WithDefault(ollama.Name),
    decide.WithMiddleware(
        decide.WithLogging(nil),    // slog.Default()
        decide.WithRecover(),       // turn a panic into an error
    ),
)
```

### Built in

**`WithLogging(logger)`** logs every call with the provider, model, question
count and elapsed time. Pass `nil` for `slog.Default()`.

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
decide.WithLogging(logger)
```

```json
{"level":"INFO","msg":"decide request","provider":"ollama","model":"nimble","questions":2,"elapsed":38120432}
```

**`WithRecover()`** converts a panic inside a provider into an error, so one bad
adapter cannot take your process down.

### Writing your own

Metrics, tracing and caching are all the same shape:

```go
func withMetrics(next decide.Handler) decide.Handler {
    return func(ctx context.Context, req decide.Request) (*decide.Result, error) {
        start := time.Now()
        res, err := next(ctx, req)

        status := "ok"
        if err != nil {
            status = "error"
        }

        decisionsTotal.WithLabelValues(providerName(ctx), req.Model, status).
            Inc()
        decisionDuration.WithLabelValues(providerName(ctx)).
            Observe(time.Since(start).Seconds())

        return res, err
    }
}

func providerName(ctx context.Context) string {
    name, _ := decide.ProviderFromContext(ctx)
    return name
}
```

`ProviderFromContext` reports which provider is serving the current call. The
client populates it before the middleware chain runs, so labels are per-backend
without you threading state through.

If you call a provider directly rather than through a client, add the name
yourself so middleware still sees it:

```go
ctx = decide.WithProviderContext(ctx, "ollama")
res, err := provider.Decide(ctx, req)
```

### Caching

Caching sits in front of the network, so the cached path skips the retry policy
too. That is usually what you want — a cache hit should be fast:

```go
func withCache(ttl time.Duration) decide.Middleware {
    store := newTTLCache(ttl)

    return func(next decide.Handler) decide.Handler {
        return func(ctx context.Context, req decide.Request) (*decide.Result, error) {
            if hit, ok := store.get(req); ok {
                return hit, nil
            }

            res, err := next(ctx, req)
            if err != nil {
                return nil, err
            }

            store.put(req, res)
            return res, nil
        }
    }
}
```

Cache on the **request**, not the result — `decide.Request` holds everything
that affects the answer. If you build requests from a struct, derive a stable
key from the fields you actually vary. Note that `Request.Extra` and `Trace` are
maps, so the usual `fmt.Sprintf` key will include Go's randomised map iteration
order; hash the fields explicitly.

## A production client

Putting it together:

```go
func newClient(logger *slog.Logger) (*decide.Client, error) {
    provider, err := openrouter.New()
    if err != nil {
        return nil, fmt.Errorf("openrouter: %w", err)
    }

    return decide.New(
        decide.WithProvider(provider),
        decide.WithProvider(ollama.New()),
        decide.WithDefault(ollama.Name),
        decide.WithRetry(decide.DefaultRetry()),
        decide.WithMiddleware(
            decide.WithLogging(logger),
            decide.WithRecover(),
        ),
    ), nil
}
```

## Next

- [Errors](errors.md) — the classifications used in `RetryOn`.
- [Testing](testing.md) — retry and middleware without a network.