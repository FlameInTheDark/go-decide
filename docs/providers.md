# Providers

A provider is a backend that sends a request to a model and returns a result.
`decide.Provider` is two methods, so adding one is small.

```go
type Provider interface {
    Name() string
    Decide(ctx context.Context, req Request) (*Result, error)
}
```

## The bundled providers

### Ollama

Local or remote, no API key by default. Requires **v0.35.0 or newer** for the
decision endpoint.

```go
provider := ollama.New()  // http://localhost:11434
```

| Option | Effect |
|---|---|
| `WithBaseURL(url)` | Point at a different server; a trailing slash is trimmed |
| `WithDefaultModel(m)` | Model used when the request leaves `Model` empty (default `nimble`) |
| `WithHTTPClient(c)` | Supply your own `*http.Client` |
| `WithHeader(k, v)` | Send a header with every request |
| `WithAPIKey(key)` | Authenticate against a hosted Ollama instance |

The address also comes from `OLLAMA_HOST` when set, so a remote server needs no
code change.

Models: `nimble` and `tev1` for text, `clef` and `clef-flash` for images.
`decide models` lists what is installed:

```bash
decide models
```

```go
names, err := provider.DecisionModelNames(ctx)   // just the usable ones
models, err := provider.Models(ctx)               // everything installed
version, err := provider.ServerVersion(ctx)
ok, err := provider.SupportsSystemOne(ctx)        // version gate
```

Each `Model` tells you what it can do:

```go
for _, m := range models {
    fmt.Println(m.ID(), m.Decision(), m.Vision(), m.ContextLength())
}
```

`FilterDecisionModels` keeps the decision-capable ones. It accepts models tagged
`decision` plus the known families (`nimble`, `tev`, `clef`, `jev`) that older
servers do not tag.

### OpenRouter

Hosted decision models, such as `typesafe/jev-1.13`.

```go
provider := openrouter.New(openrouter.WithAPIKey(key))
res, err := provider.Decide(ctx, req)
```

The key falls back to `OPENROUTER_API_KEY`. OpenRouter has no sensible default
model, so a request with an empty `Model` is rejected with `ErrInvalidRequest`.

| Option | Effect |
|---|---|
| `WithAPIKey(key)` | API key, falling back to `OPENROUTER_API_KEY` |
| `WithBaseURL(url)` | Point at a gateway instead of `openrouter.ai` |
| `WithHTTPClient(c)` | Supply your own `*http.Client` |
| `WithHeader(k, v)` | Send a header with every request |
| `WithReferer(r)` | `HTTP-Referer`, used for app rankings |
| `WithTitle(t)` | `X-OpenRouter-Title`, shown in the dashboard |

## Capabilities

Providers advertise what they can accept. `Client.DecideWith` checks the request
against these **before any network call**, so an unsupported image or an
oversized state fails locally with a precise message instead of an opaque HTTP
rejection.

```go
type Capability struct {
    Images        bool  // accepts images
    MaxQuestions  int   // 0 means unknown, so no limit
    MaxChoices    int   // options or scale levels
    MaxStateBytes int   // largest accepted state
}
```

A provider implements the optional `Capable` interface:

```go
func (p *Provider) Capabilities() decide.Capability {
    return decide.Capability{Images: true, MaxStateBytes: decide.MaxStateBytes}
}
```

Inspect one directly when you need to branch:

```go
if capable, ok := provider.(decide.Capable); ok {
    cap := capable.Capabilities()
    log.Printf("images: %t, state limit: %d bytes", cap.Images, cap.MaxStateBytes)
}
```

OpenRouter declares `Images: false`, so attaching an image to an OpenRouter
request fails locally:

```
decide: invalid request: openrouter does not accept images, but the request carries 1
```

`MaxStateBytes` is `64 KiB` on Ollama and `896 KiB` on OpenRouter — the latter
being the 1 MiB payload limit minus the room questions and framing need. A
`0` in any field means "unknown", which the client treats as no limit.

## Provider-specific fields

`Request.Extra` carries fields that have no counterpart in this package. Keys
are merged into the request body as-is, and they **override** the library's own
fields — which is how you set a parameter the library does not model yet.

```go
res, err := client.Decide(ctx, decide.Request{
    State: decide.Text("How do I rotate my API key?"),
    Questions: []decide.Question{
        decide.Choice("intent", "What does the user want?",
            decide.Options{"how_to": "Instructions", "billing": "A billing question"}),
    },
    Extra: map[string]json.RawMessage{
        "temperature": json.RawMessage("0.2"),
        "seed":        json.RawMessage("42"),
    },
})
```

Both bundled providers speak the System One dialect, so they forward these
without any further work.

For OpenRouter, `session_id`, `user` and `trace` have first-class fields
(`Request.SessionID`, `Request.User`, `Request.Trace`) and are set for you.

## Observability fields

```go
res, err := client.Decide(ctx, decide.Request{
    State:     decide.Text(ticket),
    Questions: questions,
    SessionID: "batch-2026-05-11",      // groups related requests
    User:      customerID,              // identifies the end user
    Trace: map[string]string{
        "trace_id":   requestID,
        "trace_name": "ticket-triage",
    },
})
```

## Keeping a model warm

Ollama loads a model on first use and can keep it resident between requests,
which removes the load latency from your latency budget:

```go
req := decide.Request{
    State:     decide.Text(ticket),
    Questions: questions,
    KeepAlive: 5 * time.Minute,
}
```

`KeepAlive` is a `time.Duration`. Zero omits the field so the server default
applies. A **negative** duration keeps the model loaded indefinitely — useful
for a latency-sensitive service that would rather hold the memory:

```go
req.KeepAlive = -1
```

Read it back with `KeepAliveDuration()`:

```go
if d, ok := req.KeepAliveDuration(); ok {
    fmt.Println("model stays loaded for", d)
}
```

Other providers ignore this field. Note the wire format needs a unit — the
library sends `"5m0s"`, not `"300"`, because Ollama rejects a bare number of
seconds with `time: missing unit in duration "300"`.

## Choosing between providers

`DecideWith` sends to a named provider; `Decide` uses the default set by
`WithDefault`.

```go
client := decide.New(
    decide.WithProvider(ollama.New()),
    decide.WithProvider(openrouter.New()),
    decide.WithDefault(ollama.Name),
    decide.WithRetry(decide.DefaultRetry()),
)

// Local for the common case.
res, err := client.Decide(ctx, req)

// Hosted for the hard ones.
hard := req
hard.Model = "typesafe/jev-1.13"
res, err = client.DecideWith(ctx, "openrouter", hard)
```

A straightforward fallback — try local, then hosted:

```go
func decideWithFallback(ctx context.Context, client *decide.Client, req decide.Request) (*decide.Result, error) {
    res, err := client.DecideWith(ctx, "ollama", req)
    if err == nil {
        return res, nil
    }

    // Only fall back on transient failures. A malformed request or a missing
    // API key will fail identically on the second provider.
    if !errors.Is(err, decide.ErrUnavailable) && !errors.Is(err, decide.ErrServer) {
        return nil, err
    }

    log.Printf("ollama unavailable (%v), falling back to openrouter", err)
    return client.DecideWith(ctx, "openrouter", req)
}
```

Check `decide.Registered()` or `client.Providers()` to see what is available at
runtime.

## Next

- [Custom providers](custom-providers.md) — write your own.
- [Errors](errors.md) — the kinds used in that fallback.