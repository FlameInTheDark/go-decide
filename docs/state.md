# State

State is the content the model evaluates. Every request has exactly one, and
every question is asked about it.

There are four ways to build one. Pick the narrowest that fits.

| Constructor | For | Wire shape |
|---|---|---|
| `decide.Text(s)` | Prose: a ticket, an email, a log line | JSON string |
| `decide.Object(m)` | Structured record with named fields | JSON object |
| `decide.List(items)` | Several related items | JSON array |
| `decide.JSON(v)` | Anything of your own types | whatever `v` marshals to |

## Text

The common case:

```go
req := decide.Request{
    State: decide.Text("Our checkout has returned 500 errors since 9am."),
    // ...
}
```

The zero `State` and an empty string both fail validation, so a request built
from a possibly-empty variable fails locally with `state must be a non-empty
string, object or array` rather than sending an empty prompt.

## Object

When the model needs field names to reason about, an object beats a blob of
prose. Models are markedly better at "is `severity` high?" when `severity` is
literally a field:

```go
type Ticket struct {
    Subject  string  `json:"subject"`
    Body     string  `json:"body"`
    Severity string  `json:"severity"`
    Plan     string  `json:"plan"`
}

req := decide.Request{
    State: decide.Object(map[string]any{
        "subject":  "Checkout broken",
        "body":     "Every card gets a 500 on the payment step.",
        "severity": "high",
        "plan":     "enterprise",
    }),
    // ...
}
```

Give fields names the model already understands. `severity: "high"` is
unambiguous; `priority: 2` is not.

For your own struct, `decide.JSON` is usually cleaner than hand-building a map:

```go
state, err := decide.JSON(ticket)
if err != nil {
    return err
}
req := decide.Request{State: state}
```

`JSON` accepts strings, maps, slices and structs. It **rejects** values that
marshal to a bare JSON scalar — `nil`, `42`, `true` — because the System One
endpoints require a string, object or array. It returns an error rather than
panicking, so check it:

```go
state, err := decide.JSON(42)
// err: decide: state must marshal to a string, object or array, got number
```

## List

When the decision is about the collection rather than any single item — "is
there anything in this batch that needs review?":

```go
req := decide.Request{
    State: decide.List([]any{
        map[string]any{"author": "alice", "text": "love this app"},
        map[string]any{"author": "bob", "text": "you people are worthless idiots"},
        map[string]any{"author": "carol", "text": "how do I export to CSV?"},
    }),
    Questions: []decide.Question{
        decide.Noul("needs_moderation", "Does any comment contain abuse?"),
    },
}
```

Lists get long. A hundred-item list is a lot of context and slows the decision
without making it better; batch large collections instead.

## Images

Images are attached to the **request**, not to a question, and are shared by
every question. They need a vision-capable decision model — `clef` or
`clef-flash` on Ollama.

From a file:

```go
shot, err := decide.ImageFromFile("upload.png")
if err != nil {
    return err
}

res, err := client.Decide(ctx, decide.Request{
    Model: "clef",
    State: decide.Text("What is wrong with this screenshot?"),
    Images: []decide.Image{shot},
    Questions: []decide.Question{
        decide.Noul("is_a_stack_trace", "Does this show an error message?"),
    },
})
```

From bytes you already have:

```go
img := decide.ImageFromBytes(data)
```

`ImageFromBytes` never fails, so keep `MaxImageBytes` (32 MB) in mind yourself.
`Image.Validate` checks it during normal request validation, and the provider's
capability check rejects images outright if the backend does not support them —
see [Providers](providers.md#capabilities).

The `MIMEType` is detected for logging only; the wire format carries bare
base64 without a data URL prefix.

## Inspecting a State

`State.String` renders the state as JSON, which makes states easy to log, diff
and assert on in tests:

```go
state := decide.Object(map[string]any{"plan": "enterprise"})

state.String()    // {"plan":"enterprise"}
state.IsZero()    // false
```

For a text state, `String` adds the JSON quotes. That matters when checking
size limits: a 64-byte state is 66 bytes by `len(state.String())`, which is
exactly what the capability check measures.

## Round-tripping

`State` implements `json.Marshaler` and `json.Unmarshaler`, so a state can be
stored and reloaded:

```go
encoded, err := json.Marshal(decide.Text("some ticket"))
// "\"some ticket\""

var state decide.State
err = json.Unmarshal(encoded, &state)
```

JSON numbers decode as `float64`, matching `encoding/json` defaults. If you need
ints on the way back in, decode into your own struct with `decide.JSON` instead.

## Next

- [Results](results.md) — what comes back.
- [Providers](providers.md) — size limits and what each backend accepts.