# Getting started

## Install

```bash
go get github.com/FlameInTheDark/go-decide
```

You also need a decision model to talk to. The quickest path is a local
[Ollama](https://ollama.com) server:

```bash
ollama pull nimble
```

`nimble` is the default model. `tev1` is another text model, and `clef` /
`clef-flash` handle images. Decision models require **Ollama v0.35.0 or newer**;
check with `decide models`, which fails with a clear message on older servers.

You do not have to run Ollama locally. [Providers](providers.md) covers the
hosted alternative, OpenRouter.

## Your first decision

A decision request has three parts: a **model**, some **state** to evaluate, and
**questions** to ask about it.

```go
package main

import (
	"context"
	"fmt"
	"log"

	decide "github.com/FlameInTheDark/go-decide"
	"github.com/FlameInTheDark/go-decide/providers/ollama"
)

func main() {
	ctx := context.Background()

	client := decide.New(
		decide.WithProvider(ollama.New()),
		decide.WithDefault(ollama.Name),
	)
	defer client.Close()

	res, err := client.Decide(ctx, decide.Request{
		State: decide.Text("Checkout returns 500 errors for every customer."),
		Questions: []decide.Question{
			decide.Choice("label", "Which team should own this?",
				decide.Options{
					"payments": "Payments, refunds and invoices",
					"platform": "Uptime, networking and infrastructure",
				}),
			decide.Noul("revenue_impact", "Is revenue currently blocked?"),
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	label, err := res.Choice("label")
	if err != nil {
		log.Fatal(err)
	}

	impact, err := res.Noul("revenue_impact")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%s (%.0f%%) revenue blocked: %t\n",
		label.Key, label.Probability(label.Key)*100, impact.True())
}
```

Run it:

```bash
go run .
```

```
payments (97%) revenue blocked: true
```

## The four moving parts

**`decide.Request`** is what you send. It holds the model name, the state, the
questions, and optional extras like images.

**`Question`** is one thing you want to know. Each has a name you choose, an
instruction, and a type — `choice`, `noul` or `score`. Names are the keys you
look answers up by, so pick them carefully at the start; changing them later
means changing every call site.

**`Provider`** is a backend that speaks to a model. `ollama.New()` and
`openrouter.New()` are the bundled ones. A provider interface has exactly two
methods, so writing your own is small — see
[Custom providers](custom-providers.md).

**`Result`** is what comes back: one answer per question, plus which model
answered and how many tokens it used.

## Creating a client

`decide.New` is safe for concurrent use, so build one and keep it. The two
options that matter most:

```go
client := decide.New(
    // Registers a provider instance under its own name.
    decide.WithProvider(ollama.New(ollama.WithDefaultModel("nimble"))),

    // Picks which provider Client.Decide uses. Without it you must call
    // DecideWith(ctx, "ollama", req) instead.
    decide.WithDefault(ollama.Name),
)
```

You can skip `WithProvider` entirely. Importing an adapter registers it, and
`decide.New()` resolves it by name on first use:

```go
import _ "github.com/FlameInTheDark/go-decide/providers/ollama"

client := decide.New(decide.WithDefault("ollama"))
res, err := client.Decide(ctx, req)
```

Import `github.com/FlameInTheDark/go-decide/providers/all` to register every
bundled provider at once.

`defer client.Close()` is worth writing: it releases idle connections on
providers that hold any.

## Choosing a provider per call

`DecideWith` skips the default entirely:

```go
res, err := client.DecideWith(ctx, "openrouter", req)
```

Before any network call, the client checks the request against that provider's
[capabilities](providers.md#capabilities). A request the provider cannot serve
fails locally with a precise message instead of an opaque HTTP rejection.

## Asking for a specific model

```go
res, err := client.Decide(ctx, decide.Request{
    Model:     "tev1",
    State:     decide.Text("The mobile app crashes on launch."),
    Questions: []decide.Question{decide.Noul("is_a_crash", "Does this crash the app?")},
})
```

Ollama falls back to its `DefaultModel` (`nimble`) when `Model` is empty.
OpenRouter returns `ErrInvalidRequest` if you leave it empty, because it has no
single default worth guessing.

## What to read next

- [Questions](questions.md) — the three types and how to write them well.
- [Results](results.md) — turning probabilities into decisions, which is the
  part that actually matters in production.
- [Errors](errors.md) — what to do when it does not work.