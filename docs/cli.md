# Command line

`decide` is a small CLI around the same library. It is useful for trying
questions out, checking a model, and wiring a decision into a shell script
without writing Go.

```
go install github.com/FlameInTheDark/go-decide/cmd/decide@latest
```

```
$ decide --help
NAME:
   decide - Classify text with System One decision models

USAGE:
   decide [global options] [command [command options]]

COMMANDS:
   classify, c  Ask the decision model a set of typed questions about some text
   models, m    List the decision models available on an Ollama server
   version      Print the decide version
   help, h      Shows a list of commands or help for one command
```

`classify` is the default command, so `decide "some text"` works.

## classify

Ask a set of questions about some text.

```bash
decide "Checkout returns 500 errors for everyone"
```

With no question flags it uses a demonstration question set, which is enough to
confirm your setup works:

```
label    bug       ████████████████████  0.97
         billing                          0.03
         account                          0.00
urgency  Users are affected right now     2.00
```

### Global options

| Flag | Default | Meaning |
|---|---|---|
| `--provider`, `-p` | `ollama` | `ollama` or `openrouter` |
| `--base-url` | provider default | Override the provider address |
| `--api-key`, `-k` | `$OPENROUTER_API_KEY` | Credentials for hosted providers |
| `--timeout`, `-t` | `2m` | Overall deadline |
| `--retries`, `-r` | `2` | Attempts for transient failures |
| `--verbose`, `-v` | off | Log provider requests to stderr |

### Input

The text can come from four places, checked in this order:

```bash
decide "literal text"              # positional argument
decide --state "literal text"      # explicit flag
decide --state-file ticket.txt     # a file
cat ticket.txt | decide            # stdin, when genuinely piped
```

A `.json` file is decoded into a structured state, so fields keep their names:

```bash
decide --state-file examples/ticket.json
```

```bash
decide --state-file examples/ticket.json
```

### Building questions

**Choice** — repeat `--option` with `key:description`, at least two:

```bash
decide "The API returns 403 on all requests" \
  --choice-name category \
  --choice-instructions "Which team should own this?" \
  --option "bug:Something is broken" \
  --option "billing:Payments or refunds" \
  --option "account:Login or permissions"
```

**Noul** — `--noul name:question`, refined by `--yes` and `--no`:

```bash
decide "Your app crashed" --noul needs_reply:"Does a person need to reply?" \
  --yes "The customer is waiting" --no "A robot can answer"
```

**Score** — `--scale name:question` with repeated `--level` values, lowest
first:

```bash
decide "Checkout is down" --scale urgency:"How urgent?" \
  --level "Whenever we get to it" \
  --level "Soon" \
  --level "Revenue is blocked"
```

Mix as many as you like in one command — it is still a single round trip.

### Loading questions from a file

For a stable question set, keep it in JSON. The file wraps the questions in a
`questions` key, and each entry carries only the criteria field matching its
type:

```json
{
  "questions": [
    {
      "name": "category",
      "type": "choice",
      "instructions": "Which team should own this ticket?",
      "options": { "bug": "The product is broken", "billing": "Payments or refunds" }
    },
    {
      "name": "refund",
      "type": "noul",
      "instructions": "Is the customer asking for money back?",
      "outcomes": { "false": "No refund requested", "true": "A refund is requested" }
    },
    {
      "name": "urgency",
      "type": "score",
      "instructions": "How urgent is this ticket?",
      "scale": ["Can wait", "This week", "Blocking revenue"]
    }
  ]
}
```

```bash
decide --state-file ticket.txt --questions questions.json
```

The `outcomes` object gives the noul question its two descriptions — the same
role `decide.NoulCriteria` plays in Go. Unknown fields are rejected, so a typo
like `"option"` instead of `"options"` fails loudly rather than silently
producing a question with no options.

Questions from a file go through the same validation as flags, and the CLI
reports the offending index:

```
triage.json: questions[0]: question "category": choice needs between 2 and 26 options, got 1
```

[`examples/triage.json`](../examples/triage.json) is a complete file you can copy.
Because the flags and the file both produce `[]decide.Question`, you can generate
one from Go and keep it under version control as a reviewable prompt.

### Other classify options

| Flag | Meaning |
|---|---|
| `--model`, `-m` | Decision model, e.g. `nimble`, `typesafe/jev-1.13` |
| `--image` | Attach an image file; needs a vision model (repeatable) |
| `--keep-alive` | How long Ollama keeps the model loaded, e.g. `5m`; negative keeps it loaded |
| `--session-id` | Group this request with others for observability |
| `--json`, `-j` | Print the result as JSON |
| `--no-legend` | Hide probability bars |
| `--plain` | Disable colour and styling |
| `--color` | Force colour even when not writing to a terminal |

### JSON output

`--json` prints the result in the same shape the library marshals, so it pipes
into `jq` or back into Go:

```bash
decide "Checkout returns 500" \
  --option "bug:Something is broken" --option "billing:Payments" --json \
  | jq -r '.answers.label.key'
```

```json
{
  "provider": "ollama",
  "model": "nimble",
  "answers": {
    "label": {
      "type": "choice",
      "key": "bug",
      "probabilities": { "bug": 0.965, "billing": 0.035 },
      "confidence": 0.93
    }
  },
  "usage": { "input_tokens": 412, "output_tokens": 4, "total_tokens": 416 }
}
```

This is what `--questions` reads back for a recorded decision, so an export can
be stored as a regression fixture.

### Scripting

Results go to stdout and diagnostics to stderr, so `decide ... | jq` works and
`--verbose` does not corrupt the output. Exit codes let a script branch on the
failure kind:

```bash
out=$(decide "$text" --json) || case $? in
    2) echo "bad request" >&2 ;;
    4) echo "model not installed" >&2 ;;
    5) echo "rate limited, retry later" >&2 ;;
    *) echo "provider problem" >&2 ;;
esac
```

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Unexpected or unclassified failure |
| 2 | Bad flags, bad input, invalid request |
| 3 | Missing or rejected credentials, insufficient credits |
| 4 | Unknown provider or model |
| 5 | Rate limited or timed out |
| 6 | Provider or transport failure |

## models

Query an Ollama server for installed models and report which can serve decision
requests:

```bash
decide models
```

```
ollama 0.35.1 at http://localhost:11434

MODEL            SIZ    DECISION  VISION
nimble:latest    6.2 GB  true      false
clef:latest      5.4 GB  true      true
```

This is the fastest way to check a server is new enough. On Ollama older than
v0.35.0 the command prints `warning: System One needs Ollama v0.35.0 or later`
instead of failing later with a confusing 404. With no usable models it tells
you what to install:

```
no decision models installed, run: ollama pull nimble
```

Ask about specific models instead of listing everything:

```bash
decide models nimble clef
```

```
MODEL         FOUND  DECISION  FAMILY   CONTEXT
nimble:latest  yes    true      qwen35   262144
clef:latest    yes    true      qwen35   262144
```

An unknown name is reported and exits 4, which is what makes this usable as a
pre-flight check in a deployment script:

```bash
decide models nimble >/dev/null || ollama pull nimble
```

| Flag | Effect |
|---|---|
| `--json`, `-j` | Print the model list as JSON |
| `--all` | Include models that cannot serve decisions |

The server address comes from `--base-url` or `OLLAMA_HOST`, so pointing at a
remote server needs no flag gymnastics:

```bash
OLLAMA_HOST=http://build-box:11434 decide models
```

## version

```bash
decide version
```

```
decide v1.2.3
```

Release binaries report the tag they were built from; a local `go build`
reports `dev`.

## completion

Shell completion is built in:

```bash
source <(decide completion bash)     # bash
source <(decide completion zsh)      # zsh
decide completion fish > ~/.config/fish/completions/decide.fish
decide completion powershell | Out-String | Invoke-Expression
```

## Next

- [Providers](providers.md) — what the CLI is talking to.
- [Questions](questions.md) — the JSON question file format.