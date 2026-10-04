# Documentation

A guided path through go-decide. Read the pages in order the first time; after
that jump straight to the one you need.

## Start here

| Page | What it covers |
|---|---|
| [Getting started](getting-started.md) | Install, run your first decision, the four moving parts |
| [Questions](questions.md) | Choice, noul and score questions; writing good instructions |
| [State](state.md) | Text, objects, lists and images |
| [Results](results.md) | Reading answers, probability, confidence and margin; thresholds |
| [Providers](providers.md) | Ollama and OpenRouter, capabilities, custom fields, fallbacks |
| [Errors](errors.md) | Every error the library returns and how to branch on it |
| [Retries and middleware](resilience.md) | Retry policies, logging, panic recovery, custom middleware |
| [Command line](cli.md) | The `decide` and `decide-playground` binaries, flags, exit codes |
| [Custom providers](custom-providers.md) | Implementing `decide.Provider` yourself |
| [Testing](testing.md) | Testing code that uses decide |

## Reference

- [`examples/`](https://github.com/FlameInTheDark/go-decide/tree/main/examples) —
  four complete programs: ticket triage, moderation, intent routing and auto
  tagging. Each is a real, runnable decision pipeline.
- [API documentation](https://pkg.go.dev/github.com/FlameInTheDark/go-decide) —
  generated from the source, always in sync with the code.

## The short version

Everything in this library is one call with three parts: some **state** to
evaluate, a list of **questions** to ask about it, and a provider that sends
both to a model and hands back **answers**.

```go
res, err := decide.New(
    decide.WithProvider(ollama.New()),
    decide.WithDefault(ollama.Name),
).Decide(ctx, decide.Request{
    State: decide.Text("Checkout has returned 500 errors since 9am."),
    Questions: []decide.Question{
        decide.Choice("label", "Which team owns this?",
            decide.Options{"payments": "Payments and refunds", "platform": "Uptime and infrastructure"}),
        decide.Noul("revenue_impact", "Is revenue currently blocked?"),
    },
})
if err != nil {
    return err
}

label, _ := res.Choice("label")   // label.Key == "payments"
noul, _ := res.Noul("revenue_impact")
fmt.Println(label.Key, noul.Probability)
```

That is the whole mental model. The rest of these pages are the details around
it.