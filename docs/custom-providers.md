# Custom providers

`decide.Provider` is two methods. That is the whole extension point:

```go
type Provider interface {
    Name() string
    Decide(ctx context.Context, req Request) (*Result, error)
}
```

Two contracts matter more than the signature:

1. **Implementations must be safe for concurrent use.** A `Client` is documented
   as concurrency-safe, and a shared provider is the easy place to break that.
   Guard mutable state with a mutex.
2. **Honour `ctx` cancellation.** A decision that ignores its context holds a
   connection and a goroutine for as long as the provider takes to answer.

Return a `*decide.Error` so callers can classify your failures — that is what
makes [retries and fallbacks](resilience.md) work.

## Registering one

```go
func init() {
    decide.Register(Name, func() (decide.Provider, error) {
        return New(WithAPIKey(os.Getenv("MY_API_KEY"))), nil
    })
}
```

`Register` is meant for an adapter's `init`. It panics if the same name is
registered twice, because that means two conflicting adapters.

The factory is called on first use, so importing your adapter never requires
configuration. That is what lets `decide.New()` resolve `"myapi"` by name
without an API key being present at init time.

## A complete provider

```go
package myapi

import (
    "bytes"
    "context"
    "encoding/json"
    "io"
    "net/http"
    "time"

    decide "github.com/FlameInTheDark/go-decide"
)

const Name = "myapi"

// DefaultBaseURL is where New sends requests unless an option overrides it.
const DefaultBaseURL = "https://api.example.com"

// Option configures a Provider at construction time.
type Option func(*Provider)

// WithAPIKey sets the credential sent as a bearer token.
func WithAPIKey(key string) Option {
    return func(p *Provider) { p.apiKey = key }
}

type Provider struct {
    baseURL string
    apiKey  string
    http    *http.Client
}

func New(opts ...Option) *Provider {
    p := &Provider{
        baseURL: DefaultBaseURL,
        http:    &http.Client{Timeout: 2 * time.Minute},
    }
    for _, opt := range opts {
        opt(p)
    }
    return p
}

func (p *Provider) Name() string { return Name }

// Capabilities is optional. Implementing it lets the Client reject
// impossible requests locally instead of letting the server do it.
func (p *Provider) Capabilities() decide.Capability {
    return decide.Capability{
        Images:        false,
        MaxStateBytes: 256 << 10,
    }
}

func (p *Provider) Decide(ctx context.Context, req decide.Request) (*decide.Result, error) {
    if err := req.Validate(); err != nil {
        return nil, err
    }

    body, err := json.Marshal(map[string]any{
        "model":  req.Model,
        "state":  req.State,
        "prompt": buildPrompt(req),
    })
    if err != nil {
        return nil, decide.NewError(Name, decide.KindInvalidRequest, err)
    }

    httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/decide", bytes.NewReader(body))
    if err != nil {
        return nil, decide.NewError(Name, decide.KindInvalidRequest, err)
    }
    httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
    httpReq.Header.Set("Content-Type", "application/json")

    resp, err := p.http.Do(httpReq)
    if err != nil {
        // A transport failure is almost always transient.
        return nil, decide.NewError(Name, decide.KindUnavailable, err)
    }
    defer resp.Body.Close()

    raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
    if err != nil {
        return nil, decide.NewError(Name, decide.KindDecode, err)
    }

    if resp.StatusCode != http.StatusOK {
        // ClassifyStatus maps the status; NewHTTPError parses Retry-After.
        return nil, decide.NewHTTPError(
            Name,
            resp.StatusCode,
            decodeMessage(raw),
            string(raw),
            resp.Header,
        )
    }

    return parseResult(req, raw)
}
```

The three places that matter:

- **`req.Validate()` first.** It reports every problem at once and returns a
  `*ValidationError`, which classifies as `ErrInvalidRequest`.
- **Map HTTP statuses through `decide.ClassifyStatus`.** `NewHTTPError` does
  this for you; passing the right `Kind` is what makes the default retry
  policy work.
- **Read the body with a limit.** A provider that returns an error page larger
  than you expect should not be able to exhaust your memory.

## System One providers

If your backend speaks the same dialect as Ollama's `/v1/systemone`, do not
build the body by hand:

```go
payload := req.SystemOnePayload()
body, err := json.Marshal(payload)
```

`SystemOnePayload` handles the parts that are easy to get subtly wrong —
question encoding, the state, and the `keep_alive` unit. `Request.Extra` keys
are merged over the known fields automatically.

If you need your own fields alongside them, embed the payload and write an
explicit `MarshalJSON`. **Do not rely on embedding alone**: a promoted
`MarshalJSON` from the embedded value shadows your own field set, which
silently drops everything you added.

```go
type request struct {
    decide.SystemOnePayload
    Tenant string `json:"tenant"`
}

func (r request) MarshalJSON() ([]byte, error) {
    // The type alias strips the promoted MarshalJSON from SystemOnePayload,
    // so the known fields marshal normally.
    type payload decide.SystemOnePayload

    fields := map[string]any{}
    raw, err := json.Marshal(payload(r.SystemOnePayload))
    if err != nil {
        return nil, err
    }
    if err := json.Unmarshal(raw, &fields); err != nil {
        return nil, err
    }

    fields["tenant"] = r.Tenant
    return json.Marshal(fields)
}
```

## Building answers

`decide.NewAnswers` takes a slice of `Answer` in question order, which is how
you preserve ordering from the response:

```go
func parseResult(req decide.Request, raw []byte) (*decide.Result, error) {
    var decoded struct {
        Model  string `json:"model"`
        ID     string `json:"id"`
        Prompt struct {
            TotalTokens int `json:"total_tokens"`
        } `json:"prompt_eval_count"`
        Answers map[string]struct {
            Type          string             `json:"type"`
            Key           string             `json:"key"`
            Noul          float64            `json:"noul"`
            Score         float64            `json:"score"`
            Probabilities map[string]float64 `json:"probabilities"`
            Confidence    *float64           `json:"confidence"`
            Legend        map[string]string  `json:"legend"`
        } `json:"answers"`
    }
    if err := json.Unmarshal(raw, &decoded); err != nil {
        return nil, decide.NewError(Name, decide.KindDecode, err)
    }

    answers := make([]decide.Answer, 0, len(req.Questions))
    for _, question := range req.Questions {   // request order, not map order
        got, ok := decoded.Answers[question.Name]
        if !ok {
            continue
        }

        switch question.Type {
        case decide.TypeChoice:
            answers = append(answers, decide.ChoiceAnswer{
                QuestionName:  question.Name,
                Key:           got.Key,
                Probabilities: got.Probabilities,
                Confidence:    floatValue(got.Confidence),
            })
        case decide.TypeNoul:
            answers = append(answers, decide.NoulAnswer{
                QuestionName: question.Name,
                Probability:  got.Noul,
            })
        case decide.TypeScore:
            answers = append(answers, decide.ScoreAnswer{
                QuestionName:  question.Name,
                Score:         got.Score,
                Legend:        got.Legend,
                Probabilities: got.Probabilities,
                Confidence:    floatValue(got.Confidence),
            })
        }
    }

    return &decide.Result{
        Provider: Name,
        Model:    decoded.Model,
        ID:       decoded.ID,
        Answers:  decide.NewAnswers(answers),
        Usage: decide.Usage{
            TotalTokens: decoded.Prompt.TotalTokens,
        },
        Raw: raw,
    }, nil
}
```

Iterate `req.Questions`, never the decoded map, so `Answers.All()` keeps
question order — map iteration is randomised and would make your output
unstable between runs.

Confidence arrives as a pointer because the field is optional on the wire; the
bundled providers use a small helper to turn a missing value into `0`:

```go
func floatValue(v *float64) float64 {
    if v == nil {
        return 0
    }
    return *v
}
```

## Forgetting to answer a question

If your backend cannot answer something, omit it. `Result.Missing` exists for
exactly this, and it is better than returning a zero-valued answer that reads
as a confident `0.0`:

```go
if missing := res.Missing(req.Questions); len(missing) > 0 {
    log.Printf("provider skipped %v", missing)
}
```

## Closers

If your provider holds connections or spawns goroutines, implement `Close`:

```go
func (p *Provider) Close() error {
    p.http.CloseIdleConnections()
    return nil
}
```

`Client.Close` calls it. `errors.Join` collects failures from multiple
providers, so one failing close does not hide the others.

## Using it

```go
client := decide.New(
    decide.WithProvider(myapi.New(myapi.WithAPIKey(key))),
    decide.WithDefault(myapi.Name),
    decide.WithRetry(decide.DefaultRetry()),
)
defer client.Close()
```

Or skip the wiring and let the registry resolve it:

```go
import _ "github.com/your/module/myapi"

client := decide.New(decide.WithDefault("myapi"))
```

## Not writing one

For a test double, a fake, or a one-off in-memory backend, skip the interface
entirely. `ProviderFunc` adapts a function:

```go
provider := decide.ProviderFunc{
    ID: "fake",
    Fn: func(ctx context.Context, req decide.Request) (*decide.Result, error) {
        return &decide.Result{
            Provider: "fake",
            Answers: decide.NewAnswers([]decide.Answer{
                decide.NoulAnswer{QuestionName: "urgent", Probability: 0.9},
            }),
        }, nil
    },
}
```

See [Testing](testing.md).

## Next

- [Errors](errors.md) — the kinds your provider should return.
- [Testing](testing.md) — testing an adapter without a network.