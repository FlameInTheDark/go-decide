# Questions

A question is one thing you want to know about the state. Every question has a
**name** (the key you look the answer up by), **instructions** (what to decide)
and a **type**.

There are three types. Use the one that matches the shape of the answer you
actually want, not the one you think the problem is about.

| Type | Use it when | Answer |
|---|---|---|
| [`choice`](#choice) | Picking one of a fixed set | A key plus the probability of each option |
| [`noul`](#noul) | Yes/no, with confidence | Probability of "true" |
| [`score`](#score) | Rating on an ordered scale | A number on the scale, plus per-level probabilities |

## Choice

Choose one option out of a set you define. Two to 26 options.

```go
q := decide.Choice("label", "Which category best fits this ticket?",
    decide.Options{
        "bug":       "Something is broken",
        "billing":   "Payments, invoices or refunds",
        "account":   "Login, password or permissions",
        "feature":   "A request for new functionality",
    })
```

`decide.Options` is just `map[string]string`. The **key** is a short machine
identifier you branch on; the **description** is what the model reads. Keep keys
stable and terse — they end up in logs and in your metrics.

Look the answer up by key:

```go
label, err := res.Choice("label")
if err != nil {
    return err
}

switch label.Key {          // the selected option
case "bug":
    return fileBug()
case "billing":
    return routeToBilling()
}
```

You also get the full distribution:

```go
label.Probability("bug")     // 0.93
label.Best()                 // "bug", the highest-probability key
label.Margin()               // 0.88, gap between the top two options
label.Confidence             // 0.86, how concentrated the mass is
for _, r := range label.Ranked() {
    fmt.Printf("%s: %.3f\n", r.Key, r.Probability)
}
```

### Building options incrementally

The builder methods return **copies**, so a half-built question can be a shared
template:

```go
base := decide.Choice("label", "Which category best fits this ticket?")

product := base.WithOptions(decide.Options{
    "bug":     "Something is broken in the product",
    "billing": "A billing or refund question",
})
support := base.WithOptions(decide.Options{
    "bug":     "Something is broken",
    "howto":   "A how-to or usage question",
    "other":   "Something else",
})

// Use whichever fits your call site.
questions := []decide.Question{product}
```

This is the idiomatic way to reuse question sets. Nothing is mutated, so no
locking and no surprises from shared state.

`WithOption(key, description)` adds one option; `WithOptions` adds a whole map.
Both append, so calling either twice with the same key keeps the later
description.

### Two options is a yes/no question

A choice question with exactly two options is a clumsier `noul`. Use `noul`
instead — you get a simpler answer shape and a clearer instruction to the model.

## Noul

A yes/no question. The answer is the probability that the answer is "true".

```go
q := decide.Noul("needs_human", "Does this need a person to look at it?")
```

The model only sees "No" and "Yes" unless you say what they mean, which is
usually worth doing:

```go
q := decide.Noul("needs_human",
    "Does this ticket need a human to look at it?",
    decide.NoulCriteria{
        False: "A known issue, answerable by the docs",
        True:  "Unclear, angry, or asking for something we do not offer",
    })
```

Read the answer:

```go
answer, err := res.Noul("needs_human")
if err != nil {
    return err
}

answer.Probability   // 0.72 — the raw signal
answer.True()        // probability > 0.5
answer.False()       // the complement
```

`True()` cuts at 0.5. For anything more interesting, threshold on
`Probability` yourself — see [Results](results.md#thresholds).

### Naming noul questions

The convention that reads best is a predicate: `is_urgent`, `needs_review`,
`contains_pii`. `True()` then reads as a sentence.

## Score

Rate something on an ordered scale. Levels go **lowest first**, and you need at
least two.

```go
q := decide.Score("urgency", "How urgent is this ticket?", decide.Scale{
    "Whenever we get to it",
    "Soon",
    "Users are affected right now",
    "Revenue is blocked",
})
```

Three levels is usually enough. A five-level scale makes the model split its
mass thinly and lowers confidence without adding information.

```go
answer, err := res.Score("urgency")
if err != nil {
    return err
}

answer.Score          // 1.99, the weighted average, on the scale's own range
answer.Level()        // 2 — the nearest level index
answer.Max()          // 3 — the highest valid index
answer.Description(2) // "Users are affected right now"
answer.Best()         // 2 — the most probable level
answer.Confidence     // 0.96
```

Note that `Score` is a weighted average, not a bucket. With levels `0..3` you
get a number like `1.99`, not `2`. That is the useful behaviour: it preserves
the difference between "barely level 2" and "clearly level 2". Use `Level()`
when you want a bucket.

Use `Score` when the levels have a real order and you might want to average or
threshold on the number. Use `choice` when they do not.

## Mixing types

Questions of different types mix freely in one request. This is the normal
case, and one request costs one round trip:

```go
req := decide.Request{
    State: decide.Text(ticketText),
    Questions: []decide.Question{
        decide.Choice("label", "Which category best fits?",
            decide.Options{"bug": "Something is broken", "billing": "Payments or refunds"}),
        decide.Noul("needs_reply", "Does the customer expect a reply from a person?"),
        decide.Score("urgency", "How urgent is this?", decide.Scale{"Low", "Medium", "High"}),
    },
}
```

Every question is answered from the same single pass over the state, so adding a
question is far cheaper than adding a request.

## Writing instructions that work

The instructions are the prompt. Three rules that matter more than anything else:

**Say what to decide, not how to think.** "Which category best fits this
ticket?" works. "Consider the wording, the sentiment and the product area, then
weigh them" does not — it talks the model out of a direct answer.

**Describe the options, don't just name them.** `"billing": "Payments, invoices
or refunds"` beats `"billing": "billing"`. The description is what the model
classifies against.

**One question per question.** Two questions in one instruction get one blended
probability, which you then have to unpick. Ask twice.

## Limits

A choice question needs 2 to 26 options; `MaxCriteria` is 26 because that is the
wire limit of the System One endpoints, one entry per letter of the alphabet.
Noul needs no criteria. Score needs at least 2 levels.

These are checked locally before the request leaves your process, together with
every other problem in the request — see [Errors](errors.md#validation-errors).

## Checking questions yourself

`Question.Validate` reports problems into a `*ValidationError` without sending
anything, which is useful in tests:

```go
q := decide.Choice("label", "Which label?", decide.Options{})

v := &decide.ValidationError{}
q.Validate(v)
if err := v.OrNil(); err != nil {
    log.Print(err) // question "label": choice needs between 2 and 26 options, got 0
}
```

## Next

- [State](state.md) — what you are asking about, including images.
- [Results](results.md) — reading the answers and picking thresholds.