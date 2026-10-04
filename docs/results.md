# Results

A `*decide.Result` holds one answer per question, plus which model answered and
what it cost.

```go
type Result struct {
    Provider      string
    Model         string
    UpstreamModel string
    Upstream      string
    ID            string
    Answers       Answers
    Usage         Usage
    Raw           json.RawMessage   // the unmodified provider response
}
```

## Getting an answer

Ask for the type you expect:

```go
label, err := res.Choice("label")   // ChoiceAnswer
urgent, err := res.Noul("urgent")   // NoulAnswer
score, err := res.Score("urgency")  // ScoreAnswer
```

These are safe on a nil `Result` — they return `ErrNotFound` instead of
panicking — and they return a `KindDecode` error if the question was answered
with a different type, so a rename that breaks a call site shows up as an error
rather than a silent zero value.

For the type-agnostic path, use `Answer`:

```go
answer, err := res.Answer("label")   // decide.Answer interface

switch a := answer.(type) {
case decide.ChoiceAnswer:
    fmt.Println(a.Key)
case decide.NoulAnswer:
    fmt.Println(a.Probability)
case decide.ScoreAnswer:
    fmt.Println(a.Score)
}
```

Or iterate everything, which is handy in generic reporting code:

```go
for _, answer := range res.Answers.All() {
    fmt.Println(answer.Name(), answer.Type(), answer.String())
}
```

`Answers` also answers "did the model actually answer everything I asked?":

```go
if missing := res.Missing(req.Questions); len(missing) > 0 {
    log.Printf("model skipped %v", missing)
}
```

## Thresholds

This is the part that matters. **A decision model gives you probabilities, not
decisions.** Turning those into an action is a business decision, and it belongs
in your code, not in a prompt.

`answer.True()` cuts at 0.5 because that is a reasonable default and a terrible
threshold. For anything that costs money, sends a message to a customer, or
deletes something, set the threshold yourself:

```go
// Thresholds decide when the model is trusted to act without review.
const (
    autoRouteMargin = 0.35  // smallest gap between top two labels
    refundMinProb   = 0.80  // how sure before flagging a refund
    pageAtScore     = 1.50  // urgency at which to page someone
)
```

### Choice: threshold on margin, not confidence

`Confidence` says how concentrated the probability mass is. `Margin` says how
far ahead the winner is. **Margin is the one to threshold on** — a model can be
highly confident and still genuinely torn between two similar categories.

```go
label, err := res.Choice("label")
if err != nil {
    return err
}

if label.Margin() < autoRouteMargin {
    // Too close to call. Send it to a human, or to a more specific question.
    return routeToHuman(label)
}

return autoRoute(label.Key)
```

### Noul: threshold on the probability you care about

```go
refund, err := res.Noul("refund_appropriate")
if err != nil {
    return err
}

if refund.Probability >= refundMinProb {
    return flagForBilling(refund.Probability)
}
```

`True()` is `Probability >= 0.5`. That reads well for a boolean your code
stores as a boolean; use the raw probability when you care how sure it is.

### Score: threshold on the number

`Score` is a weighted average over the levels, on the scale's own range. With
three levels you get 0, 1 or 2 and fractions between:

```go
urgency, err := res.Score("urgency")
if err != nil {
    return err
}

if urgency.Score >= pageAtScore {
    pageOncall()
}

fmt.Println("urgency level", urgency.Level(), "of", urgency.Max())
```

Use `Level()` for a bucket and `Score` when the intermediate values carry
information you want to average over or sort by.

### The three-way decision

The pattern that comes up most often in production is not "yes or no" but "act,
escalate, or drop":

```go
label, err := res.Choice("label")
if err != nil {
    return err
}

switch {
case label.Margin() < 0.15:
    // The model cannot tell these apart. Ask a sharper question, or fall
    // back to a rule.
    return useHeuristic(label)
case label.Confidence < 0.5:
    return queueForReview(label)
default:
    return autoRoute(label.Key)
}
```

This is the design that stops a decision model from making you look careless:
the model is good at the easy 90%, the code is honest about the remaining 10%.

## Confidence versus probability

They are different numbers and mixing them up is a common bug.

- `Probability` is the answer to your question: "how likely is this a bug?"
- `Confidence` describes the **shape** of the distribution: how much mass sits on
  one option, from 0 (uniform across all options) to 1 (one option takes almost
  everything).

A 0.4 probability can come with 0.95 confidence (the model is sure it is 40/60)
or 0.2 confidence (the model is genuinely split 40/60 between your two best
options). Threshold on `Probability` to answer the question; look at
`Confidence` to know whether to trust it.

`Margin` is the most stable signal of the three, because it only depends on the
gap between the top two options.

## Usage

```go
res.Usage.InputTokens    // prompt tokens consumed
res.Usage.OutputTokens   // tokens generated
res.Usage.TotalTokens    // when the provider reports it
res.Usage.Cost           // USD, only on providers that meter
```

Ollama reports tokens and leaves `Cost` at zero. OpenRouter reports cost. This
is where the cost-per-decision number comes from when you are deciding whether
to run a model on every request or only on the interesting ones.

## Exporting and reloading

`Result` marshals and unmarshals, so a decision can be stored as JSON and read
back — for auditing, for golden-file tests, or for replaying a batch offline:

```go
encoded, err := json.Marshal(res)
if err != nil {
    return err
}

var replay decide.Result
err = json.Unmarshal(encoded, &replay)

label, err := replay.Choice("label")   // works exactly like a fresh result
```

`Answers.UnmarshalJSON` visits keys in sorted order, so a reloaded result
iterates deterministically. That is what makes golden-file tests on a whole
`Result` possible.

The shape:

```json
{
  "provider": "ollama",
  "model": "nimble",
  "answers": {
    "label": {
      "type": "choice",
      "key": "bug",
      "probabilities": {"bug": 0.96547, "billing": 0.03339, "account": 0.00113},
      "confidence": 0.8588
    },
    "revenue_impact": {"type": "noul", "noul": 0.016},
    "urgency": {
      "type": "score",
      "score": 1.99,
      "legend": {"0": "Low", "1": "Medium", "2": "High"},
      "probabilities": {"0": 0.004, "1": 0.006, "2": 0.99},
      "confidence": 0.9589
    }
  },
  "usage": {"input_tokens": 862, "output_tokens": 4, "total_tokens": 866}
}
```

Note the `noul` key — that is how the wire format spells the probability of
"true". `UnmarshalJSON` also accepts `probability` as an alias, so hand-written
fixtures in either spelling work.

`Raw` holds the untouched provider response and is excluded from JSON, so
exporting a result does not leak provider internals. Read it directly when
debugging a decoding issue.

## Next

- [Errors](errors.md) — when the call does not return a result at all.
- [Testing](testing.md) — asserting on results without a model.