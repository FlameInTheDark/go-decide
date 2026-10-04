# Testing

Most code that uses `decide` makes decisions, and a decision model is
non-deterministic by nature. Two rules keep tests fast and meaningful:

- **Test your logic, not the model's opinion.** Put the branching behind a seam
  and test the seam.
- **Reserve live calls for a small number of end-to-end checks**, opt-in behind
  an environment variable.

## Faking a provider

`ProviderFunc` adapts a function into a provider, so no interface
implementation is needed:

```go
func stub(answers ...decide.Answer) decide.Provider {
    return decide.ProviderFunc{
        ID: "stub",
        Fn: func(_ context.Context, req decide.Request) (*decide.Result, error) {
            return &decide.Result{
                Provider: "stub",
                Model:    req.Model,
                Answers:  decide.NewAnswers(answers),
            }, nil
        },
    }
}
```

Wire it into a client and your whole application runs offline:

```go
client := decide.New(
    decide.WithProvider(stub(
        decide.ChoiceAnswer{
            QuestionName:  "team",
            Key:           "billing",
            Probabilities: map[string]float64{"billing": 0.9, "bug": 0.1},
            Confidence:    0.8,
        },
    )),
    decide.WithDefault("stub"),
)
defer client.Close()
```

Because the stub echoes `req.Model` and sees `req.Questions`, you can still
assert on the request that was built:

```go
var seen decide.Request

provider := decide.ProviderFunc{
    ID: "recorder",
    Fn: func(_ context.Context, req decide.Request) (*decide.Result, error) {
        seen = req
        return &decide.Result{Provider: "recorder"}, nil
    },
}

client := decide.New(
    decide.WithProvider(provider),
    decide.WithDefault("recorder"),
)

req := decide.Request{
    Model:     "test-model",
    State:     decide.Text("something broke"),
    Questions: []decide.Question{decide.Choice("team", "Who owns this?")},
}
_, err := client.Decide(context.Background(), req)
if err != nil {
    t.Fatal(err)
}

if seen.Model != "test-model" {
    t.Errorf("model = %q", seen.Model)
}
if len(seen.Questions) != 1 {
    t.Errorf("questions = %d, want 1", len(seen.Questions))
}
```

## Failing on purpose

To test your error handling, return a real `*decide.Error`:

```go
rateLimited := decide.ProviderFunc{
    ID: "flaky",
    Fn: func(_ context.Context, _ decide.Request) (*decide.Result, error) {
        return nil, decide.NewError("flaky", decide.KindRateLimited, errors.New("slow down"))
    },
}
```

Then assert the classification your code branches on:

```go
_, err := client.Decide(context.Background(), req)
if !errors.Is(err, decide.ErrRateLimited) {
    t.Fatalf("got %v", err)
}
```

## Injecting failures on purpose

Middleware wraps a `Handler`, which makes it the natural seam for failing a
specific test without touching the provider:

```go
func failWith(kind decide.Kind) decide.Middleware {
    return func(next decide.Handler) decide.Handler {
        return func(ctx context.Context, req decide.Request) (*decide.Result, error) {
            return nil, decide.NewError("test", kind, errors.New("injected"))
        }
    }
}
```

`decide.WithMiddleware` applies middleware in order, so the first one listed is
the outermost:

```go
client := decide.New(
    decide.WithProvider(stub()),
    decide.WithDefault("stub"),
    decide.WithMiddleware(failWith(decide.KindServer)),
)
```

## Testing the wire format

Providers have their own tests, and the useful trick is an `httptest` server
that captures the body:

```go
func TestRequestBody(t *testing.T) {
    var got map[string]any

    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
            t.Fatal(err)
        }
        w.Header().Set("Content-Type", "application/json")
        io.WriteString(w, `{"answers":{}}`)
    }))
    defer server.Close()

    provider := ollama.New(ollama.WithBaseURL(server.URL))
    _, _ = provider.Decide(context.Background(), decide.Request{
        Model:     "m",
        State:     decide.Text("hello"),
        Questions: []decide.Question{decide.Choice("label", "Which label?")},
    })

    payload, _ := got["questions"].([]any)
    if len(payload) != 1 {
        t.Errorf("questions = %v", got["questions"])
    }
}
```

Assert on things that actually broke in practice: that `keep_alive` carries a
unit (`"5m0s"`, never `"300"` — Ollama rejects bare seconds), and that your own
fields survive alongside `SystemOnePayload`.

## Live tests

Live tests are opt-in so `go test ./...` stays fast and hermetic. The bundled
ones check for an opt-in variable and skip otherwise:

```go
func TestLiveDecide(t *testing.T) {
    if os.Getenv("DECIDE_LIVE_OLLAMA") == "" {
        t.Skip("set DECIDE_LIVE_OLLAMA=1 to run")
    }

    client := decide.New(
        decide.WithProvider(ollama.New()),
        decide.WithDefault("nimble"),
        decide.WithRetry(decide.DefaultRetry()),
    )
    defer client.Close()

    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()

    res, err := client.Decide(ctx, decide.Request{
        State: decide.Text("Checkout returns 500 for every customer"),
        Questions: []decide.Question{
            decide.Choice("label", "Which label fits this text?",
                decide.Options{"bug": "Something is broken"},
                decide.Options{"billing": "Payments or refunds"},
            ),
        },
    })
    if err != nil {
        t.Fatal(err)
    }

    label, err := res.Choice("label")
    if err != nil {
        t.Fatal(err)
    }
    // Assert on structure, not on the exact label: the model's opinion
    // is allowed to change between releases.
    if label.Confidence <= 0 {
        t.Errorf("confidence = %v", label.Confidence)
    }
}
```

```bash
DECIDE_LIVE_OLLAMA=1 go test ./providers/ollama -run TestLive -v -count=1
```

Assert on things the library guarantees — no error, an answer present, usage
recorded, confidence above zero — and not on which label wins. If you need
golden files for regression detection, record them and review diffs by hand:
a decision model update will legitimately change them.

One warning: if you run tests through a wrapper that does not forward the
environment (`task`, some IDE runners), the live tests silently skip and still
report green. Run `go test` directly when you expect them to execute.

## Determinism in retries

`RetryPolicy.Rand` is yours to set, which makes jitter reproducible:

```go
policy := decide.RetryPolicy{
    MaxAttempts: 3,
    BaseDelay:   10 * time.Millisecond,
    MaxDelay:    time.Second,
    Rand:        rand.New(rand.NewSource(1)),
}
```

## Next

- [Resilience](resilience.md) — retries, timeouts and fallback strategies.
- [Custom providers](custom-providers.md) — testing an adapter you wrote.