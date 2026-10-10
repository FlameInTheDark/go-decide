# UX and usability review

A review of `go-decide` as a **library a Go developer integrates** and as a **product a
human uses** (the `decide` CLI and the `decide-playground` web UI).

Scope: every committed file — the root package, both provider adapters, the CLI, the
playground server, the React frontend, the docs, the examples and the build tooling.

Method: read-through of all 12.9k lines of Go and 2.7k lines of TypeScript. Every item
below cites the file and line it came from. **Caveat:** the review sandbox has no Go
toolchain, so nothing here was verified by running `go build` / `go test` — the findings
are source-level, and the ones marked *verify* deserve a runtime check before you act.

Severity:

| Level | Means |
|---|---|
| **High** | Wrong or broken behaviour a user will hit in a normal flow |
| **Medium** | Confusing, inconsistent, or an easy mistake to make |
| **Low** | Polish, ergonomics, documentation drift |

---

## Contents

1. [Library usability](#1-library-usability)
2. [Errors, retries and resilience](#2-errors-retries-and-resilience)
3. [Command line UX](#3-command-line-ux)
4. [Playground server](#4-playground-server)
5. [Web UI](#5-web-ui)
6. [Docs, examples and tooling](#6-docs-examples-and-tooling)
7. [Strengths worth preserving](#7-strengths-worth-preserving)
8. [Suggested order of work](#8-suggested-order-of-work)

---

## 1. Library usability

### 1.1 Transport failures are not `*decide.Error` — the single biggest gap `High`

`internal/httpx/sender.go:92-95` turns every transport failure into a plain
`fmt.Errorf`:

```go
resp, err := s.doer().Do(req)
if err != nil {
    return nil, fmt.Errorf("decide: %s request failed: %w", s.Provider, err)
}
```

Consequences, all of them user-visible:

- **`errors.Is(err, decide.ErrUnavailable)` is false** when Ollama is not running — the
  most common failure of all. `docs/errors.md:44` lists `ErrUnavailable` as "Could not
  reach the provider at all", and `docs/errors.md:190-200` shows custom providers
  producing exactly that, but the bundled adapters never do.
- **Retries silently do nothing.** `retry.go:122-128` (`isRetryable`) requires a
  `*decide.Error`; a plain error is never retried. `--retries 2` and
  `RetryPolicy{MaxAttempts: 3}` are no-ops for connection-refused, DNS failures, resets
  and TLS errors — i.e. for precisely the failures retries exist for.
- **The CLI exits 1, not 6.** `cmd/decide/settings.go:239-262` maps by sentinel, so an
  unreachable server lands in `exitFailure`. `docs/cli.md:175` documents 6 as
  "Provider or transport failure"; scripts written against that table mis-behave.
- **The playground shows a 500 with an empty `kind`** (`internal/playground/errors.go:60-62`)
  instead of a 503 `unavailable`, so the UI cannot say "start Ollama".

`decide.NewError` already classifies timeouts (`errors.go:211-216`) and is used
everywhere else in the adapters — it is simply never reached on this path.

**Fix:** in `Sender.do`, wrap transport errors with `decide.NewError(s.Provider,
decide.KindUnavailable, err)` (which upgrades timeouts to `KindTimeout` automatically).
This one change fixes the sentinels, the retries, the exit codes and the playground
status in a single stroke.

### 1.2 `Client` never calls `Request.Validate()` `Medium`

`client.go:214-236` (`DecideWith`) runs capability checks and retries, but validation
lives inside each adapter (`providers/ollama/ollama.go:184`, `providers/openrouter/openrouter.go:154`).
The README promises "Requests are validated locally before any network call" — true for
the bundled adapters, **false for any custom provider that forgets the call**. It is also
duplicated work today: `checkCapabilities` and `Validate` both walk the questions.

**Fix:** call `req.Validate()` once in `DecideWith` before the retry loop. Keep it in the
adapters too (a provider used standalone still wants it), but make the client the
guarantee. `docs/custom-providers.md:147` already tells authors to validate first; the
client should not have to trust them.

### 1.3 Capability checks ignore two of the five advertised limits `Medium`

`client.go:245-271` checks `Images`, `MaxQuestions`, `MaxStateBytes` and
`MaxChoices` — but:

- `Capability.MaxImageBytes` (`provider.go:41-42`) is **never checked anywhere**, even
  though Ollama advertises it (`providers/ollama/ollama.go:178`) and the UI displays it.
- `MaxChoices` is only compared against `question.Options` (`client.go:262-267`); a
  **score question's `Scale` is exempt**, so a 40-level scale passes locally and is
  rejected by the server.

**Fix:** add the image-size loop and check `len(question.Scale)` with the same rule.

### 1.4 `Ranked()` is non-deterministic when probabilities tie `High`

`answer.go:386-395`:

```go
for key, prob := range probabilities {   // map iteration: random order
    ranked = append(ranked, Ranked{Key: key, Probability: prob})
}
sort.SliceStable(...)                     // stable, so ties keep the random order
```

Two options at 0.50/0.50 come back in a random order on every run. This affects
`ChoiceAnswer.Ranked()` and `ScoreAnswer.Ranked()`, therefore the CLI legend
(`cmd/decide/output.go:175-181`) and every playground bar
(`internal/playground/answers.go`). It also makes golden-file tests on a result flaky in
a way that is maddening to debug, because it only shows up on exact ties.

**Fix:** tie-break on `Key` (score levels must compare numerically, not
lexicographically, or level 10 sorts before level 2).

### 1.5 Score answers are keyed by index strings `Medium`

`ScoreAnswer.Probabilities` and `Legend` are `map[string]float64` / `map[string]string`
keyed by `"0"`, `"1"`, `"2"` (`answer.go:100-103`), because that is the wire format.
Consumers must write `a.Probabilities[strconv.Itoa(level)]` and round-trip through
`Atoi` to sort. `internal/playground/answers.go` and `web/src/lib/*` both contain
index-string plumbing that a typed accessor would remove.

**Fix:** add `func (a ScoreAnswer) LevelProbability(level int) float64` and
`func (a ScoreAnswer) Levels() []int` (sorted), and document the string keys as the wire
representation rather than the API.

### 1.6 `ScoreAnswer.Max()` does not mean what its doc says `Medium`

`answer.go:119-132` — the doc comment says "the number of criteria minus one", but the
implementation scans the keys of `Probabilities`:

```go
func (a ScoreAnswer) Max() int {
    if len(a.Probabilities) == 0 { return 0 }
    max := 0
    for key := range a.Probabilities { if idx := indexOf(key); idx > max { max = idx } }
    return max
}
```

If a provider omits a zero-probability level, `Max()` under-reports and the CLI prints
nonsense: `levelLabel` (`cmd/decide/output.go:235-240`) renders `level 2 of 1`. Related:
`Level()` (`answer.go:135`) is `int(math.Round(a.Score))` with no clamping to
`[0, Max()]`, so an out-of-range score yields a level with no legend entry.

**Fix:** derive `Max()` from the legend (falling back to the probability keys) and clamp
`Level()` to `[0, Max()]`.

### 1.7 There is no exported way to decode a question set from JSON `Medium`

`Question.Name` is `json:"-"` and `UnmarshalJSON` deliberately does not restore it
(`question.go:47`, `question.go:282-319`). That is correct for the wire format (the name
is the map key), but it means **no exported type can round-trip a full question set**, so
every consumer re-invents one:

- `cmd/decide/questions.go:38-54` defines `questionFile` / `fileQuestion`.
- `internal/playground/api.go:19-30` defines `QuestionInput` / `OptionInput`.
- `web/src/lib/types.ts:13-21` defines `QuestionInput` a third time.

Three structs, three chances to drift, ~150 duplicated lines.

**Fix:** export a small `decide.QuestionSet` type with `MarshalJSON` /
`UnmarshalJSON` / `Validate()` and have all three consumers use it. It makes the file
format documented in `docs/cli.md:150-180` a first-class part of the API instead of a
convention three packages happen to share.

### 1.8 Option order is lost `Low`

`Options` is `map[string]string` (`question.go:69`), so `decide.Choice(...)`, the JSON
payload and the CLI's `--json` export all scramble the author's ordering. The model does
not care, but humans reviewing an exported prompt do, and the playground already needed
an `options_list` escape hatch (`internal/playground/api.go:26-27`).

**Fix:** keep an ordered key slice alongside the map; emit it in `MarshalJSON`; accept
either shape on the way in.

### 1.9 Small ergonomics `Low`

| Item | Where | Note |
|---|---|---|
| `ChoiceAnswer.Probability(key)` returns 0 for an unknown key | `answer.go:46-48` | Indistinguishable from a real 0.0. Add `Has(key)` or `(float64, bool)`. |
| `Result.Answer` on a nil result returns the bare `ErrNotFound` sentinel, while `Answers.Get` returns a `*decide.Error` | `result.go:54-59` vs `answer.go:190-196` | Two shapes for one condition. |
| No package-level `decide.Decide(ctx, req)` or `decide.NewWith(provider)` | — | The one-call case needs three lines and a `WithDefault`. |
| `Register` panics on a duplicate name (`client.go:24-32`) with no `Unregister` | `client.go` | Painful in tests that build several clients; consider returning an error, or add `Unregister` under a `testing` note. |
| `NoulAnswer.True()` doc says "exceeds the false threshold of 0.5" but the code is `>= 0.5` | `answer.go:85-87` | `docs/results.md` has it right; fix the Go comment. |
| `Capability.MaxStateBytes` is compared against the state alone | `client.go:274` | Ollama enforces a 64 KiB **body** (`providers/ollama/ollama.go:46-51`), so a state just under the limit can still 413 once questions and framing are added. Same shape in OpenRouter (`providers/openrouter/openrouter.go:34-37`). Note the approximation in the messages. |

---

## 2. Errors, retries and resilience

### 2.1 `Error.Is` compares sentinels by identity `Medium` (documented, still a trap)

`errors.go:186-192` and `errors.go:292`:

```go
func (e *Error) Is(target error) bool { sentinel := e.Kind.sentinel(); return sentinel != nil && sentinel == target }
```

Anyone who wraps a `decide.Error` with `fmt.Errorf("...: %w", err)` keeps `errors.As`
working but **loses `errors.Is`**, because `errors.Is` unwraps to the `*decide.Error` and
asks it, and the wrapper's target is never the sentinel. `docs/errors.md:96-104` explains
this, but it is a rule people will not read until it bites.

**Fix:** keep identity comparison **and** fall back to `errors.Is(e.Err, target)` so a
wrapped sentinel still matches. Cheap, and it removes a subtle trap.

### 2.2 `RetryPolicy.Do` returns a stale result alongside an error `Low`

`retry.go:72-74`: `return result, err` on a non-retryable failure. Callers that write
`res, err := ...; if err != nil { return err }` are fine, but anyone who inspects `res`
first gets a non-nil, partially populated value on a failure. Return `nil, err` for
consistency with the rest of the package.

### 2.3 Retries do not distinguish "gave up" from "not retryable" `Low`

`retry.go` returns the last error with no attempt count. After three failed attempts
against a flaky server the caller cannot log "tried 3 times" or expose it in the UI (the
playground would love to show it next to the Retry button).

**Fix:** add `Attempts int` to `Error`, or export a `RetriesExhausted` wrapper.

### 2.4 `ValidationError` has no field anchors `Low`

`ValidationError.Problems` is `[]string` (`errors.go:278-281`). The playground needs
per-field anchors, so it defines its own `problem` type with an `Anchor`
(`internal/playground/api.go:106-131`) and re-parses the message text. Structured
problems (`{Field, Message}`) would let both the playground and any form UI highlight
the offending input without string surgery.

---

## 3. Command line UX

### 3.1 `decide models` silently ignores `--provider` and `--api-key` `High`

`cmd/decide/models.go:68-73` builds an Ollama provider directly:

```go
provider := ollama.New(
    ollama.WithBaseURL(or(cfg.baseURL, ollama.DefaultBaseURL)),
    ollama.WithHTTPClient(httpClientFor(cfg.timeout)),
)
```

`decide models -p openrouter` (or `-k $KEY`) still queries `localhost:11434`, and the
output header says `ollama 0.x at http://localhost:11434` with no hint that the flag was
ignored. `docs/cli.md:214-217` documents `models` as an Ollama-only command, but the
global flag table (`docs/cli.md:36-43`) presents `--provider` as universal.

**Fix:** either honour the selected provider (and error with "this provider does not
expose a model list" for OpenRouter) or reject the combination with a clear usage error.
Silence is the only wrong answer.

### 3.2 `make run` classifies the path as text `Medium`

`Makefile:67`: `go run ./cmd/decide examples/ticket.json`. Positional arguments are
joined into the **state string** (`cmd/decide/settings.go:136-139`), so this classifies
the literal text `examples/ticket.json`. The intent is clearly `--state-file`.

**Fix:** `go run ./cmd/decide --state-file examples/ticket.json`.

### 3.3 No way to check a request without spending a token `Medium`

The playground has `POST /api/validate` (`internal/playground/handlers.go:83-121`); the
CLI has no equivalent. Building a long `--option`/`--level` command means running it for
real to find out whether it is well-formed.

**Fix:** add `--dry-run` that runs `req.Validate()` plus `decide.CheckCapabilities` and
prints the payload it would send, then exits 0/2. It is a handful of lines and it makes
the CLI scriptable in CI.

### 3.4 Defaults contradict the "nothing is filled in behind your back" promise `Low`

The README (line ~126) says: "Nothing is filled in behind your back: a `--scale` without
`--level` is an error rather than a silent built-in rubric." But:

- `--choice-instructions` defaults to `"Which label fits this text?"` and
  `--choice-name` to `label` (`cmd/decide/classify.go:71-79`), so `--option a:x
  --option b:y` invents the question text.
- With **no** question flags at all, three demonstration questions appear
  (`cmd/decide/classify.go:289-292`, `demoQuestions`).

Both are documented in `--help`, and the demo set is genuinely useful on a first run. The
fix is cosmetic: when defaults are applied, say so in the output — e.g. print
`using the demonstration question set; pass --help to see the question flags` on stderr
once per run.

### 3.5 Smaller CLI items `Low`

| Item | Where | Note |
|---|---|---|
| `--retries` usage says "how many attempts to make" but it is attempts *after* the first (`MaxAttempts = Retries + 1`) | `cmd/decide/main.go:58-63`, `internal/cliconfig/config.go:106` | Say "retries after the first attempt". |
| No `--user`, `--trace`, `--extra` flags even though `Request` carries them and the playground exposes them | `cmd/decide/classify.go` | Hosted-provider observability is unreachable from scripts. |
| No config file or `DECIDE_MODEL` / `DECIDE_PROVIDER` env vars; only `OPENROUTER_API_KEY` and `OLLAMA_HOST` are read | `providers/openrouter/openrouter.go:103`, `providers/ollama/ollama.go:155-163` | A `~/.config/decide/config.toml` would remove a lot of repeated flags. |
| `decide models` prints a warning on an old server, but `docs/getting-started.md:17` says it "fails with a clear message" | `cmd/decide/models.go:112-114` | Align the doc or the behaviour. |
| Duplicate fenced block for `--state-file examples/ticket.json` | `docs/cli.md:52-58` | Delete one. |
| No `--quiet` | — | `--json` output is the only machine path that suppresses the header. |

---

## 4. Playground server

### 4.1 The placeholder-frontend warning never fires `High`

`web/embed.go:38-45` detects the placeholder by checking that `index.html` does **not**
reference `/assets/`:

```go
return !strings.Contains(string(data), "/assets/")
```

But the committed placeholder (`web/dist/index.html:11-12`) *does* reference
`/assets/index-vTbdnss1.js` and `/assets/index-C13fNBmn.css` — files that do not exist
(`git ls-files web/dist` lists only `index.html`). So `IsPlaceholder()` returns `false`
and `cmd/decide-playground/main.go:188-190` stays silent.

The user's experience of `go run ./cmd/decide-playground` on a fresh clone: the browser
loads `/`, gets the shell, requests the bundle, `assetHandler.handle` misses
(`internal/playground/assets.go:36-39`), and the catch-all serves **HTML with a
`text/javascript` content type** (`server.go:161-173`). Blank page, no warning, nothing
on stderr.

**Fix:** make the check meaningful — verify the referenced asset actually exists (e.g.
`fs.Stat` the first `/assets/` path found), or embed an empty `dist/assets/.gitkeep` and
have `IsPlaceholder` test for it. Better still, make the placeholder page itself say
"run `make web`" so the failure is self-explanatory even if detection regresses.

### 4.2 The default port is privileged `High` *(verify on Linux)*

`cmd/decide-playground/main.go:103` defaults to `127.0.0.1:842`. Ports below 1024 are
privileged on Linux and macOS, so a non-root user gets
`listen tcp4 127.0.0.1:842: bind: permission denied` on the documented happy path
(`README.md:232`, `docs/cli.md:320`). The `listenError` unwrapper
(`main.go:122-139`) at least surfaces the cause, but a user has to know to pass
`--listen 127.0.0.1:8420`.

**Fix:** default to `:8420` (or `127.0.0.1:0` and print the resolved URL). Keep the
mnemonic, lose the `sudo`.

### 4.3 Everything is in-memory, including the work `Medium`

`internal/playground/handlers.go:219-230` keeps settings in memory; history lives in
`internal/playground/history.go`; nothing is written to disk. That is a deliberate,
documented privacy property (`README.md:236-238`) and worth keeping — but as written it
also means **a reload destroys the question set you just built**, and there is no export
button. `web/src/hooks/use-playground.ts` has no `localStorage` and
`web/src/components/results-panel.tsx` has no download affordance.

**Fix:** add explicit, opt-in persistence in the browser only (`localStorage` for the
draft + history, a "Download JSON" button next to the JSON tab, and a file picker). The
server can stay disk-free and the README's promise holds.

### 4.4 The API key is sent on every request but never clearable `Low`

`web/src/components/settings-dialog.tsx:154-166` sends `api_key` in the request body from
the browser; `internal/playground/server.go:210-237` merges it into in-memory settings
and the config endpoint only reports `api_key_set`. There is no way to revoke or clear a
key once entered (it survives until restart), and it travels in a POST body rather than a
header.

**Fix:** add a "clear key" action and a `DELETE /api/config/key`. Low risk, and it makes
the "kept in memory only" claim something a user can act on.

### 4.5 Smaller server items `Low`

| Item | Where | Note |
|---|---|---|
| `openRouterDefaultBaseURL` is duplicated as a literal to avoid importing the provider | `internal/playground/json.go:58-60` | A comment explains it, but importing `openrouter.DefaultBaseURL` costs nothing and cannot drift. |
| No request concurrency limit or per-request cancellation | `internal/playground/handlers.go:134-189` | A slow model plus an impatient user means N stacked decisions. One in-flight flag with a 409 would do. |
| `/api/history` and `/api/config` are unauthenticated for any page that can set the guard header | `server.go:144-154` | The header only stops *simple* cross-origin requests; a hostile page can still preflight-blocked-read nothing, which is the intent — but it can still trigger `/api/decide` via a form POST with a `Content-Type` it cannot set. Worth a note in the code that the guard is CSRF-shaped, not auth. |

---

## 5. Web UI

### 5.1 Attached images are silently dropped when the request is edited `High`

`web/src/App.tsx:41-59` builds the request from `questions`, `state` and `pasted`;
images live in `RequestPanel`'s local state and are pushed up through
`onRequestChange` (`web/src/components/request-panel.tsx:127-147`), which sets `pasted`.

But every builder interaction clears `pasted`:

- editing the state: `App.tsx:242-245` → `setPasted(null)`
- editing any question: `App.tsx:123-126` → `setPasted(null)`
- loading a template: `App.tsx:176`
- Clear: `App.tsx:165-170`

So: attach two images → tweak one word of the state → the images are **gone from the
request** while the Images tab (`request-panel.tsx:148-153`) still renders them from its
own local state. The user sees images attached, validation passes, and the model never
receives them.

**Fix:** lift `images` into `App` state (alongside `questions` and `state`) so it
participates in `requestFrom` and survives edits, and clear it only via "Clear" or the
per-image remove button.

### 5.2 The image size limit shown is the state limit `Medium`

`web/src/App.tsx:249-252`:

```tsx
limits={{
  max_questions: playground.validate?.capabilities?.max_questions,
  max_image_bytes: playground.validate?.capabilities?.max_state_bytes,   // ← state, not image
}}
```

`request-panel.tsx:65` then enforces it as the per-file image cap. With Ollama
(`max_state_bytes` = 64 KiB, `max_image_bytes` = 32 MiB) every image over 64 KiB is
rejected with "exceeds 0 MiB limit". The correct field is
`capabilities.max_image_bytes`, which the API already returns
(`internal/playground/errors.go` / `server.go:247-261`) and `web/src/lib/types.ts:130`
already types.

### 5.3 No keyboard path for the primary action `Medium`

There is no `keydown` handler anywhere in `web/src` (verified by grep), so:

- **Run** is mouse-only. `Cmd/Ctrl+Enter` is the expected shortcut in every
  request-builder UI (Postman, Insomnia, GraphQL playgrounds).
- **Reordering questions and scale levels** is drag-only
  (`web/src/hooks/use-drag-reorder.ts`), with no keyboard alternative and no `aria`
  semantics for the drag handle. `ListEditor` does provide up/down buttons
  (`web/src/components/list-editor.tsx:95-117`), but `QuestionEditor` does not — its
  `onMove` prop exists (`question-editor.tsx:47`) and is never wired to a control.

### 5.4 The reorder handle is mislabelled and collapses the card `Medium`

`web/src/components/question-editor.tsx:76-85`:

```tsx
<Button aria-label={`Reorder question ${index + 1}`} disabled={total < 2} onClick={() => onToggle()}>
  <GripVertical />
</Button>
```

The label promises reordering; the click toggles expand/collapse — the same action as the
adjacent header button (`:87-92`), which is also the accessible control. A screen-reader
user hears two "reorder" affordances that both expand the card, and the only real
reordering mechanism (drag) is unusable without a mouse.

**Fix:** label it "Collapse/expand question N" (or `aria-hidden` it, since the header
duplicates it), and add explicit move-up/move-down buttons like `ListEditor`'s.

### 5.5 Model selection degrades without explanation `Medium`

`web/src/components/settings-dialog.tsx:110-126`: the model `<Select>` renders only when
`models.models.length > 0`, otherwise a free-text `<Input>`. When Ollama is not running
the list is empty, so the dropdown silently becomes a text box. `modelsError` **is**
rendered (`settings-dialog.tsx:139-141`) but sits under the Base URL field, far from the
model input, and reads like a URL problem rather than "no models because the server is
unreachable".

**Fix:** render the model error next to the model field, and put a one-line
"start Ollama or pick a model manually" hint where the dropdown would be.

### 5.6 Smaller UI items `Low`

| Item | Where | Note |
|---|---|---|
| No result export | `results-panel.tsx` | The raw JSON is viewable (`code-block.tsx` has a copy button), but there is no "download as JSON" for use as a `decide --questions` file or a test fixture. |
| History entries cannot be renamed, pinned or filtered | `history-panel.tsx` | Newest-first list only; fine at 20 runs, painful at 200. |
| Validation is debounced 400 ms with a spinner but no "looks good" state | `App.tsx:131-141` | The status bar shows problems; a positive confirmation would reduce "is it valid yet?" re-checks. |
| `Restore` from history replaces the draft with no confirmation | `App.tsx:150-158` | One click can discard an edited request; an undo or confirm is cheap. |
| The state editor is a plain `Textarea` with a JSON placeholder | `request-panel.tsx:180-195` | The JSON tab loads Monaco, but the field most likely to contain JSON does not get validation feedback until the server responds. |
| Dark theme is hardcoded | `web/dist/index.html:2-8`, `use-theme.ts` | Deliberate and documented; noting it so it is not read as an oversight. |
| `web/README.md` is still the Vite template boilerplate | `web/README.md` | It documents React Compiler and Oxlint options that have nothing to do with this project. Replace with build/dev/proxy instructions (`vite.config.ts` already proxies to `:842`). |

---

## 6. Docs, examples and tooling

### 6.1 Docs describe behaviour the code does not have `Medium`

| Claim | Where | Reality |
|---|---|---|
| "Could not reach the provider at all → `ErrUnavailable`" | `docs/errors.md:44` | Only if a provider wraps it; the bundled adapters do not (§1.1). |
| Exit code 6 = "Provider or transport failure" | `docs/cli.md:175` | Transport failures exit 1 (§1.1). |
| "Requests are validated locally before any network call" | `README.md` (Errors) | Depends on the provider calling `Validate()` (§1.2). |
| `decide models` "fails with a clear message on older servers" | `docs/getting-started.md:17` | It prints a warning and exits 0 (`cmd/decide/models.go:112-114`). |
| `ScoreAnswer.Max()` is "the number of criteria minus one" | Go doc, `answer.go:119-120` | Derived from the probability keys (§1.6). |
| `NoulAnswer.True()` "exceeds" 0.5 | Go doc, `answer.go:85-86` | `>= 0.5`. |

Each is a sentence-level fix, but together they are the difference between a library
people trust at 2am and one they have to read the source of.

### 6.2 `MaxStateBytes` is easy to trip over and hard to diagnose `Medium`

`decide.MaxStateBytes` is 64 KiB (`request.go:20-21`) and OpenRouter's is 896 KiB
(`providers/openrouter/openrouter.go:34-37`). Passing a large document is one of the
first things a new user tries. The error says "state is N bytes, ollama accepts at most
65536" only when the provider advertises the capability; otherwise it surfaces as
`request body is N bytes, the limit is M` from `ollama.go:202-205`.

**Fix:** add a short "Limits" section to `docs/state.md` (it currently has none) covering
text vs image bodies, per-provider numbers, and the `--state-file` structured-state
workaround.

### 6.3 CI does not build the Go binaries `Low`

`.github/workflows/ci.yml` runs `go vet`, `go test` and `gofmt` on the Go side; only
`release.yml` builds binaries. A compile error in `cmd/` that `vet` does not catch (it
usually does, but not always — e.g. a `//go:build` mistake or an embed path) waits for
release day. Add `go build ./...` to the test job; it costs seconds.

### 6.4 Examples are strong; two small gaps `Low`

`examples/` is genuinely good — four runnable programs, thresholds as named constants, a
`README` that explains the *habits* rather than the mechanics. Two gaps:

- None of them demonstrate a **custom provider** or `decide.ProviderFunc`, which
  `docs/testing.md:13-40` recommends as the primary testing seam.
- None demonstrate **`Middleware`** end to end (caching, metrics), though
  `docs/resilience.md` documents it.

---

## 7. Strengths worth preserving

Worth naming explicitly, because these are the things a refactor could easily undo:

- **One interface, two adapters, zero configuration to swap.** `Provider` is two methods
  (`provider.go:12-18`) and `decide.Register` + lazy lookup
  (`client.go:133-141`) mean importing a package is the whole setup.
- **Errors are classified, not string-matched.** Sentinels, `Kind`, `errors.Is`/`As`,
  `Retryable()`, `RetryDelay()` — this is the right model, and §1.1 is the only place the
  implementation does not live up to it.
- **Validation collects every problem at once** (`errors.go:276-327`) instead of stopping
  at the first, including duplicate-name detection across the whole request.
- **Capabilities fail fast** (`client.go:245-271`, exported as `CheckCapabilities` so a
  UI can pre-validate).
- **The CLI refuses to block on stdin** unless it is genuinely a pipe
  (`cmd/decide/main.go:72-79`, `cmd/decide/settings.go:183-200`), and the reasoning is
  written down in the source.
- **The playground is a thin shell over `decide.Client`**, not a parallel
  implementation — same client, same validation, same errors
  (`internal/playground/handlers.go:239-244`).
- **The playground refuses to bind a routable address** and explains why
  (`cmd/decide-playground/main.go:217-239`).
- **Docs have a point of view** — "threshold on margin, not confidence", "the model is
  good at the easy 90%, the code is honest about the remaining 10%" (`docs/results.md`)
  is advice most libraries do not give.

---

## 8. Suggested order of work

**Week 1 — correctness (small diffs, large effect)**

1. Wrap transport errors in `decide.NewError` (`internal/httpx/sender.go:94`). Fixes
   sentinels, retries, exit codes and playground status at once.
2. Tie-break `rank()` on key (`answer.go:386-395`).
3. Fix `IsPlaceholder` detection, or embed a self-explaining placeholder
   (`web/embed.go:38-45`).
4. Change the default `--listen` to an unprivileged port.
5. Lift playground images into `App` state.
6. `max_image_bytes: capabilities.max_image_bytes` (`App.tsx:251`).

**Week 2 — API surface**

7. `Client.DecideWith` calls `req.Validate()`.
8. Complete the capability checks (image bytes; score `Scale` vs `MaxChoices`).
9. `ScoreAnswer.Max()` from the legend + clamp `Level()`; add `LevelProbability(int)`.
10. Export `QuestionSet` and collapse the three duplicate question-file structs.
11. Add `--dry-run` to the CLI; make `models` honour `--provider` (or refuse it).
12. `Error.Is` falls back to `errors.Is(e.Err, target)`.

**Week 3 — experience**

13. `Cmd/Ctrl+Enter` to run; move-up/down buttons for questions; fix the grip label.
14. Playground: `localStorage` draft + history, export/import JSON, clear-API-key.
15. Model-field error placement and an explicit "server unreachable" hint.
16. Docs pass: the six mismatches in §6.1, plus a "Limits" section in `docs/state.md`.
17. `Makefile` `run` target; `go build ./...` in CI; replace `web/README.md`.
