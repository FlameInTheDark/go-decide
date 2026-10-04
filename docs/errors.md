# Errors

Everything the library returns is either a `*decide.Error` or a
`*decide.ValidationError`. Both are designed to be classified, not string-matched.

## The shape of an error

```go
type Error struct {
    Provider   string          // "ollama"
    Kind       Kind            // provider-independent classification
    StatusCode int             // HTTP status, 0 for transport failures
    Message    string          // provider-supplied message
    Body       string          // raw response body, truncated
    RetryAfter time.Duration   // what the provider asked for
    Err        error           // the underlying cause
}
```

Reach it with `errors.As`:

```go
var dErr *decide.Error
if errors.As(err, &dErr) {
    log.Printf("%s failed: %s (HTTP %d)", dErr.Provider, dErr.Kind, dErr.StatusCode)
}
```

## Kinds

`Kind` is the classification to branch on. It is provider-independent, so the
same code works whether the failure came from a local server or a hosted API.

| Sentinel | Kind | Usually means |
|---|---|---|
| `ErrInvalidRequest` | `KindInvalidRequest` | Your request is malformed — fix the code |
| `ErrAuth` | `KindAuth` | Missing or rejected credentials |
| `ErrPayment` | `KindPayment` | Out of credits, billing problem |
| `ErrForbidden` | `KindForbidden` | Authenticated but not allowed |
| `ErrNotFound` | `KindNotFound` | Unknown model, or the model is not installed |
| `ErrPayloadTooLarge` | `KindPayloadTooLarge` | State or image over the limit |
| `ErrRateLimited` | `KindRateLimited` | Too many requests |
| `ErrServer` | `KindServer` | The provider failed on its side (5xx) |
| `ErrUnavailable` | `KindUnavailable` | Could not reach the provider at all |
| `ErrTimeout` | `KindTimeout` | The request took too long |
| `ErrDecode` | `KindDecode` | The response did not parse |
| `ErrUnsupported` | `KindUnsupported` | The provider cannot do this |

`ErrNotFound` on Ollama usually means the model is not pulled. The message says
so: `model "nope" not found, try pulling it first: is the decision model
installed? try: ollama pull nimble`.

## errors.Is

Match a sentinel:

```go
if errors.Is(err, decide.ErrRateLimited) {
    // back off
}
```

The sentinel is compared **by identity**, which means a wrapper around a
sentinel does not match. `errors.Is(fmt.Errorf("context: %w", decide.ErrRateLimited), decide.ErrRateLimited)`
is true, but a `*decide.Error` whose `Kind` is `KindRateLimited` is the thing
that matches `ErrRateLimited` directly.

When you want a category rather than a specific condition, compare `Kind`:

```go
var dErr *decide.Error
if errors.As(err, &dErr) {
    switch dErr.Kind {
    case decide.KindRateLimited, decide.KindTimeout, decide.KindUnavailable:
        return retryLater()
    case decide.KindAuth, decide.KindPayment:
        return fixCredentials(dErr)
    default:
        return err
    }
}
```

`ClassifyStatus(status)` maps an HTTP code onto a `Kind` if you are writing your
own adapter.

## Deciding what to retry

`Retryable` answers whether the same request could plausibly succeed later:

```go
var dErr *decide.Error
if errors.As(err, &dErr) && dErr.Retryable() {
    // worth another attempt
}
```

`Temporary()` is an alias of the same thing, for compatibility with older code.

`RetryDelay()` returns what the provider asked for via `Retry-After`, which is
zero when it did not ask:

```go
if d := dErr.RetryDelay(); d > 0 {
    time.Sleep(d)
}
```

## Validation errors

Problems that would be rejected by every provider — an empty state, a choice
question with one option, a duplicate question name, an oversized image — are
caught locally, before any network call.

They arrive as a `*ValidationError`, which collects **every** problem instead of
stopping at the first:

```go
var vErr *decide.ValidationError
if errors.As(err, &vErr) {
    for _, problem := range vErr.Problems {
        log.Print(problem)
    }
}
```

```
state must be a non-empty string, object or array
question "label": choice needs between 2 and 26 options, got 1
duplicate question name "label"
```

That matters when you are generating questions programmatically: fixing one
problem at a time is miserable when all of them are already known.

`errors.Is(err, decide.ErrInvalidRequest)` is true for a validation error, and
`errors.As` with `**decide.Error` also succeeds, so existing code that branches
on the invalid-request classification keeps working.

### Putting it together

```go
func classify(err error) string {
    // Validation problems are bugs in the caller, so list them all.
    var vErr *decide.ValidationError
    if errors.As(err, &vErr) {
        return "invalid request: " + strings.Join(vErr.Problems, "; ")
    }

    var dErr *decide.Error
    switch {
    case errors.As(err, &dErr) && dErr.Kind == decide.KindNotFound:
        return "install the model first"
    case errors.Is(err, decide.ErrAuth):
        return "check your credentials"
    case errors.Is(err, decide.ErrRateLimited):
        return "too many requests, try again shortly"
    case errors.Is(err, decide.ErrUnavailable), errors.Is(err, decide.ErrTimeout):
        return "the provider is unreachable"
    default:
        return err.Error()
    }
}
```

## Cancelling

Context cancellation surfaces as `ctx.Err()`, so check it before treating an
error as a provider failure:

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

res, err := client.Decide(ctx, req)
if errors.Is(err, context.DeadlineExceeded) {
    return errTooSlow
}
```

## Writing errors in a custom provider

Build them with the constructors so callers can classify your failures:

```go
func (p *Provider) decide(ctx context.Context, req decide.Request) (*decide.Result, error) {
    resp, err := p.post(ctx, req)
    if err != nil {
        // A transport-level failure is usually transient.
        return nil, decide.NewError(Name, decide.KindUnavailable, err)
    }

    if resp.StatusCode != http.StatusOK {
        // An HTTP failure, with the status, the provider's message and the body.
        return nil, decide.NewHTTPError(
            Name,
            resp.StatusCode,
            decodeMessage(resp.Body),
            string(resp.Body),
            resp.Header,
        )
    }

    return parseResult(req, resp.Body)
}
```
```

`NewHTTPError` parses `Retry-After` into `Error.RetryAfter` for you. Wrapping is
fine — `Error.Unwrap` exposes the cause to `errors.Is` and `errors.As`.

## Next

- [Retries and middleware](resilience.md) — acting on these classifications.
- [Custom providers](custom-providers.md) — producing these errors.