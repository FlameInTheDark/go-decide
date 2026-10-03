# Examples

Runnable programs showing how decision models are used in production. Each one
is a complete `main` package, so it can be read top to bottom and run as-is.

They need a local decision model:

```sh
ollama pull nimble
```

| Example | Problem | Shows |
| --- | --- | --- |
| [`ticket-triage`](ticket-triage) | routing support tickets to a team and priority | `choice` + `noul` + `score`, thresholds, refusing to auto-route when uncertain |
| [`moderation`](moderation) | applying a content policy | one question per rule, per-rule thresholds, block/escalate/allow |
| [`intent-router`](intent-router) | routing chatbot messages | `choice.Margin()` to detect ambiguity and ask instead of guessing |
| [`autotag`](autotag) | deriving a tag set from content | independent binary questions, per-tag thresholds, structured JSON state |

## Run them

```sh
go run ./examples/ticket-triage -text "Checkout returns 500 for everyone"

echo "buy cheap followers now" | go run ./examples/moderation

go run ./examples/intent-router -text "how do I rotate my API key"

go run ./examples/autotag -file ticket.json
```

## What they have in common

The interesting part of any decision-model integration is not the request, it is
what you do with the probabilities afterwards. Each example keeps its
thresholds in plain Go constants at the top of the file, because "how confident
is confident enough" is a business decision, not a modelling one.

They also share a few habits worth copying:

- **Ask everything in one request.** One model load answers all questions.
- **Validate locally first.** Bad criteria never reach the network, and the
  error lists every problem at once via `*decide.ValidationError`.
- **Check capabilities for free.** `decide.Client` rejects an image for a
  text-only provider before any request is sent.
- **Keep the state meaningful.** `decide.Object` gives the model structured
  context; see [`autotag`](autotag) reading a JSON file.
- **Branch on errors with `errors.Is`**, never on error strings.
- **Never guess when uncertain.** `Margin()` below a threshold means a human
  decides, not the model.

[`ticket-triage`](ticket-triage) shows the error handling in full:

```go
switch {
case errors.Is(err, decide.ErrNotFound):
	return fmt.Errorf("%w: try: ollama pull nimble", err)

case errors.Is(err, decide.ErrInvalidRequest):
	var vErr *decide.ValidationError
	if errors.As(err, &vErr) {
		for _, problem := range vErr.Problems {
			log.Printf("  - %s", problem)
		}
	}
	return err

case errors.Is(err, decide.ErrRateLimited):
	return fmt.Errorf("%w: the local server is busy, try again shortly", err)
}
```

## Data files

- [`triage.json`](triage.json) — a questions file for `decide classify --questions`
- [`ticket.json`](ticket.json) — a structured state for `decide classify --state-file`

## Recording the demos

The `*.tape` files in this directory record the CLI GIFs in the README with
[VHS](https://github.com/charmbracelet/vhs). Warm the model first, or the first
inference may not finish inside the recorded window:

```sh
go build -o decide.exe ./cmd/decide
./decide.exe "warm up" -noul "ok:Is it ok?"

vhs examples/classify.tape
vhs examples/custom.tape
vhs examples/questions.tape
```