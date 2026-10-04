package playground_test

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	decide "github.com/FlameInTheDark/go-decide"
	"github.com/FlameInTheDark/go-decide/internal/cliconfig"
	"github.com/FlameInTheDark/go-decide/internal/playground"
	"github.com/FlameInTheDark/go-decide/providers/ollama"
	"github.com/FlameInTheDark/go-decide/providers/openrouter"
)

// stubName is the provider the stub factory registers under.
const stubName = "playground-stub"

func init() {
	decide.Register(stubName, func() (decide.Provider, error) { return &stubProvider{}, nil })
}

// stubProvider answers a decision locally so the decide and history paths can
// be tested without a model. Its answer is fixed, so a test that expects a
// probability can rely on it.
type stubProvider struct{}

func (p *stubProvider) Name() string { return stubName }

func (p *stubProvider) Decide(_ context.Context, req decide.Request) (*decide.Result, error) {
	// Answer whatever was asked, so a test's expectations follow from its
	// own request rather than from a fixture nobody can extend.
	answers := make([]decide.Answer, 0, len(req.Questions))
	for _, question := range req.Questions {
		switch question.Type {
		case decide.TypeChoice:
			probabilities := make(map[string]float64, len(question.Options))
			share := 1.0 / float64(len(question.Options))
			for key := range question.Options {
				probabilities[key] = share
			}
			first := ""
			for key := range question.Options {
				if first == "" || key < first {
					first = key
				}
			}
			answers = append(answers, decide.ChoiceAnswer{
				QuestionName:  question.Name,
				Key:           first,
				Probabilities: probabilities,
				Confidence:    0.6,
			})
		case decide.TypeScore:
			probabilities := make(map[string]float64, len(question.Scale))
			for level := range question.Scale {
				probabilities[strconv.Itoa(level)] = 1 / float64(len(question.Scale))
			}
			legend := make(map[string]string, len(question.Scale))
			for level, text := range question.Scale {
				legend[strconv.Itoa(level)] = text
			}
			answers = append(answers, decide.ScoreAnswer{
				QuestionName:  question.Name,
				Score:         0,
				Probabilities: probabilities,
				Legend:        legend,
				Confidence:    0.6,
			})
		case decide.TypeNoul:
			// A question named "skipme" gets no answer, so the
			// missing-name path has something to report.
			if question.Name == "skipme" {
				continue
			}
			answers = append(answers, decide.NoulAnswer{
				QuestionName: question.Name,
				Probability:  0.2,
			})
		}
	}

	return &decide.Result{
		Model:   "stub",
		Raw:     json.RawMessage(`{"stub":true}`),
		Answers: decide.NewAnswers(answers),
	}, nil
}

func (p *stubProvider) Capabilities() decide.Capability {
	return decide.Capability{MaxStateBytes: decide.MaxStateBytes, MaxChoices: decide.MaxCriteria}
}

func newStubServer(t *testing.T) *playground.Server {
	t.Helper()
	return newTestServer(t, cliconfig.Settings{Provider: stubName, Timeout: 2 * time.Second})
}

// guardHeader must match the server's requirement: a browser page cannot add
// this header cross-origin without a preflight, so it is what stops another
// site from driving the local server.
const guardHeader = "X-Decide-Playground"

func testAssets() fs.FS {
	return fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<!doctype html><html><body>app</body></html>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log(1)")},
	}
}

func newTestServer(t *testing.T, settings cliconfig.Settings) *playground.Server {
	t.Helper()
	server, err := playground.New(playground.Options{
		Settings: settings,
		Assets:   testAssets(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return server
}

func do(t *testing.T, server *playground.Server, method, path, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	// Every /api route is guarded, so the header is what a real browser
	// sends. Tests that exercise the guard itself set it explicitly.
	req.Header.Set(guardHeader, "1")
	resp, err := server.Test(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// doRaw sends a request without any headers, for tests that care about the
// guard itself.
func doRaw(t *testing.T, server *playground.Server, method, path string) *http.Response {
	t.Helper()
	resp, err := server.Test(httptest.NewRequest(method, path, nil))
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

// mustBody reads a whole response body, for assertions about the raw JSON.
func mustBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}

// containsAny reports whether want is present in got.
func containsAny(got []string, want string) bool {
	for _, entry := range got {
		if strings.Contains(entry, want) {
			return true
		}
	}
	return false
}

// TestNewRequiresAssets guards the one hard requirement of the package: without
// an embedded frontend there is nothing to serve, and failing at construction
// beats serving a 404 on every page.
func TestNewRequiresAssets(t *testing.T) {
	if _, err := playground.New(playground.Options{Settings: cliconfig.Settings{}}); err == nil {
		t.Fatal("expected an error when no assets are supplied")
	}
}

// TestAPIGuardsAgainstCrossOriginCalls is the security property the custom
// header exists for. A hostile page cannot set a custom header without a
// preflight, and this server never answers one, so every /api call from a
// browser context carries the header while a forged one does not.
func TestAPIGuardsAgainstCrossOriginCalls(t *testing.T) {
	server := newTestServer(t, cliconfig.Settings{})

	for _, path := range []string{"/api/config", "/api/models", "/api/history"} {
		resp := doRaw(t, server, http.MethodGet, path)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s without the guard header = %d, want 403", path, resp.StatusCode)
		}
	}

	resp := do(t, server, http.MethodGet, "/api/config", "")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/api/config with the guard header = %d, want 200", resp.StatusCode)
	}
}

// TestConfigNeverEchoesTheAPIKey is the other half of the security posture: the
// UI has to be able to show that a key is set without the key ever crossing
// the wire back to the browser.
func TestConfigNeverEchoesTheAPIKey(t *testing.T) {
	server := newTestServer(t, cliconfig.Settings{
		Provider: openrouter.Name,
		APIKey:   "sk-secret-value",
	})

	view := decode[playground.ConfigView](t, do(t, server, http.MethodGet, "/api/config", ""))
	if !view.APIKeySet {
		t.Error("APIKeySet = false, want true")
	}

	body := mustBody(t, do(t, server, http.MethodGet, "/api/config", ""))
	if strings.Contains(body, "sk-secret-value") {
		t.Error("the config response leaked the API key")
	}
}

// TestConfigReportsDefaults keeps the UI honest about what is really being
// called: an unset base URL is reported as the provider's own default rather
// than an empty box the user has to guess about.
func TestConfigReportsDefaults(t *testing.T) {
	tests := []struct {
		name     string
		settings cliconfig.Settings
		wantURL  string
	}{
		{"ollama default", cliconfig.Settings{Provider: ollama.Name}, ollama.DefaultBaseURL},
		{"openrouter default", cliconfig.Settings{Provider: openrouter.Name}, "https://openrouter.ai"},
		{"explicit url wins", cliconfig.Settings{Provider: ollama.Name, BaseURL: "http://box:11434"}, "http://box:11434"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, tt.settings)
			view := decode[playground.ConfigView](t, do(t, server, http.MethodGet, "/api/config", ""))
			if view.BaseURL != tt.wantURL {
				t.Errorf("BaseURL = %q, want %q", view.BaseURL, tt.wantURL)
			}
		})
	}
}

// TestAssetsAreCachedCorrectly pins the caching rules: the shell must never be
// cached or a rebuilt binary would keep serving the old app, while hashed
// assets are immutable and safe to cache forever.
func TestAssetsAreCachedCorrectly(t *testing.T) {
	server := newTestServer(t, cliconfig.Settings{})

	tests := []struct {
		path     string
		wantCode int
		wantCC   string
	}{
		{"/", http.StatusOK, "no-store"},
		{"/decisions", http.StatusOK, "no-store"}, // a client-side route
		{"/assets/app.js", http.StatusOK, "public, max-age=31536000, immutable"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp := do(t, server, http.MethodGet, tt.path, "")
			if resp.StatusCode != tt.wantCode {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantCode)
			}
			if got := resp.Header.Get("Cache-Control"); got != tt.wantCC {
				t.Errorf("Cache-Control = %q, want %q", got, tt.wantCC)
			}
		})
	}
}

// TestValidateReportsEveryProblemAtOnce is the whole point of validating
// locally: a form must be able to show every issue in one pass rather than
// making the user fix them one round trip at a time.
func TestValidateReportsEveryProblemAtOnce(t *testing.T) {
	server := newTestServer(t, cliconfig.Settings{})

	body := `{
		"state": "",
		"questions": [
			{"name": "first", "type": "choice", "options": {"only": "Only"}},
			{"name": "second", "type": "mystery", "instructions": "?"},
			{"name": "", "type": "choice", "instructions": "?", "options": {"a": "A", "b": "B"}}
		]
	}`

	resp := do(t, server, http.MethodPost, "/api/validate", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: validation is a successful answer", resp.StatusCode)
	}

	got := decode[playground.ValidateResponse](t, resp)
	if got.OK {
		t.Error("OK = true, want false")
	}
	if len(got.Problems) < 3 {
		t.Errorf("got %d problems, want at least 3: %v", len(got.Problems), got.Problems)
	}
	for _, want := range []string{"options", "unknown type", "name"} {
		if !containsAny(got.Problems, want) {
			t.Errorf("no problem mentions %q: %v", want, got.Problems)
		}
	}
}

// TestValidateAnchorsProblemsToFields lets the UI highlight the offending row
// instead of dumping a sentence at the user.
func TestValidateAnchorsProblemsToFields(t *testing.T) {
	server := newTestServer(t, cliconfig.Settings{})

	body := `{
		"state": "hello",
		"questions": [
			{"name": "good", "type": "choice", "instructions": "?", "options": {"a": "A", "b": "B"}},
			{"name": "bad", "type": "choice", "options": {"a": "A", "b": "B"}}
		]
	}`

	got := decode[playground.ValidateResponse](t, do(t, server, http.MethodPost, "/api/validate", body))
	if got.OK {
		t.Fatal("OK = true, want false")
	}
	if len(got.Fields) == 0 {
		t.Fatal("no field anchors reported")
	}
	if got.Fields[0] != "questions[1]" {
		t.Errorf("Fields[0] = %q, want questions[1]", got.Fields[0])
	}
	// The healthy question still comes back so the form can render it.
	if len(got.Questions) != 1 || got.Questions[0].Name != "good" {
		t.Errorf("Questions = %+v, want just the healthy one", got.Questions)
	}
}

// TestValidateReportsCapabilities lets the UI warn about an impossible request
// before spending anything.
func TestValidateReportsCapabilities(t *testing.T) {
	server := newTestServer(t, cliconfig.Settings{Provider: ollama.Name})

	body := `{"state":"hello","questions":[{"name":"a","type":"choice","instructions":"?","options":{"x":"X","y":"Y"}}]}`
	got := decode[playground.ValidateResponse](t, do(t, server, http.MethodPost, "/api/validate", body))

	if !got.OK {
		t.Fatalf("OK = false, problems: %v", got.Problems)
	}
	if !got.Capabilities.Known {
		t.Error("Capabilities.Known = false, want true for ollama")
	}
	if got.Capabilities.MaxStateBytes != decide.MaxStateBytes {
		t.Errorf("MaxStateBytes = %d, want %d", got.Capabilities.MaxStateBytes, decide.MaxStateBytes)
	}
}

// TestValidateRejectsAnOversizedStateBeforeSending proves the capability check
// runs on the validate path, so the user learns the state is too large without
// a round trip to a model that will refuse it anyway.
func TestValidateRejectsAnOversizedStateBeforeSending(t *testing.T) {
	server := newTestServer(t, cliconfig.Settings{Provider: ollama.Name})

	state, err := json.Marshal(strings.Repeat("x", decide.MaxStateBytes+1))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := `{"state":` + string(state) + `,"questions":[{"name":"a","type":"choice","instructions":"?","options":{"x":"X","y":"Y"}}]}`

	got := decode[playground.ValidateResponse](t, do(t, server, http.MethodPost, "/api/validate", body))
	if got.OK {
		t.Fatal("OK = true, want false for an oversized state")
	}
	if !containsAny(got.Problems, "at most") {
		t.Errorf("problems = %v, want the provider's size limit", got.Problems)
	}
}

// TestMalformedJSONIsAClientError keeps a broken payload distinguishable from a
// provider failure.
func TestMalformedJSONIsAClientError(t *testing.T) {
	server := newTestServer(t, cliconfig.Settings{})

	for _, path := range []string{"/api/validate", "/api/decide"} {
		resp := do(t, server, http.MethodPost, path, `{"state":`)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400", path, resp.StatusCode)
		}
	}
}

// The UI reads the provider's HTTP status out of the error body, under the key
// "status". It arrived as "status_code", so the panel showed "kind=not_found"
// with nothing beside it.
func TestAnErrorViewCarriesTheProviderStatus(t *testing.T) {
	view := playground.ErrorView{
		Message:    `decide ollama: not_found (HTTP 404): model "nope" not found`,
		Kind:       "not_found",
		Provider:   "ollama",
		StatusCode: 404,
	}

	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got, ok := decoded["status"]; !ok {
		t.Fatalf("the status key is missing from %s", encoded)
	} else if got != float64(404) {
		t.Errorf("status = %v, want 404", got)
	}
	if _, ok := decoded["status_code"]; ok {
		t.Errorf("the wire key is status_code, not status: %s", encoded)
	}
}

// Every derived number the UI shows must come from a library method, so the
// playground can never disagree with a Go caller reading the same result.
func TestDecideReturnsTheAnswerWithEveryDerivedNumber(t *testing.T) {
	server := newStubServer(t)

	got := decode[playground.DecideResponse](t, do(t, server, http.MethodPost, "/api/decide", `{
		"state": "a ticket about billing",
		"questions": [{"name": "label", "type": "choice", "instructions": "?", "options": {"a": "billing", "b": "other"}}]
	}`))

	if len(got.Answers) != 1 {
		t.Fatalf("answers = %d, want 1", len(got.Answers))
	}
	answer := got.Answers[0]
	if answer.Key != "a" {
		t.Errorf("key = %q, want a", answer.Key)
	}
	// The stub splits its probability evenly, so the margin between the top
	// two of them is zero. That is the library's own Margin, not the UI's.
	if math.Abs(answer.Margin) > 1e-9 {
		t.Errorf("margin = %v, want 0", answer.Margin)
	}
	if math.Abs(answer.Confidence-0.6) > 1e-9 {
		t.Errorf("confidence = %v, want 0.6", answer.Confidence)
	}
	if len(answer.Probabilities) != 2 {
		t.Fatalf("probabilities = %d, want 2", len(answer.Probabilities))
	}
	if got.Questions[0].Name != "label" {
		t.Errorf("question name = %q, want label", got.Questions[0].Name)
	}
	if got.Raw == nil {
		t.Error("the provider response is missing")
	}
}

// An answer the model did not produce still belongs in the response, named, so
// the UI can say which question went unanswered instead of dropping it.
func TestDecideAnswersCarryMissingNames(t *testing.T) {
	server := newStubServer(t)

	got := decode[playground.DecideResponse](t, do(t, server, http.MethodPost, "/api/decide", `{
		"state": "a ticket",
		"questions": [
			{"name": "label", "type": "choice", "instructions": "?", "options": {"a": "billing", "b": "other"}},
			{"name": "skipme", "type": "noul", "instructions": "?", "outcomes": {"false": "no", "true": "yes"}}
		]
	}`))

	if !containsAny(got.Missing, "skipme") {
		t.Errorf("missing = %v, want it to name the unanswered question", got.Missing)
	}
}

// A negative noul answer is the interesting half of the pair, so the "true"
// flag has to survive serialization. With omitempty on the bool it disappeared
// exactly when it was false, and a client reading answer.true saw undefined
// rather than a plain false.
func TestANegativeNoulStillReportsFalse(t *testing.T) {
	server := newStubServer(t)
	resp := do(t, server, http.MethodPost, "/api/decide", `{
		"state": "charged twice",
		"questions": [
			{"name": "label", "type": "choice", "instructions": "pick", "options": {"a": "A", "b": "B"}},
			{"name": "refund", "type": "noul", "instructions": "refund asked?", "outcomes": {"false": "no", "true": "yes"}}
		]
	}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var decoded struct {
		Answers []map[string]json.RawMessage `json:"answers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}

	var refund map[string]json.RawMessage
	for _, answer := range decoded.Answers {
		if string(answer["name"]) == `"refund"` {
			refund = answer
		}
	}
	if refund == nil {
		t.Fatalf("no refund answer in %d answers", len(decoded.Answers))
	}
	raw, ok := refund["true"]
	if !ok {
		t.Fatal(`the "true" key is missing, so false reads as unknown rather than no`)
	}
	if got := string(raw); got != "false" {
		t.Errorf("true = %s, want false", got)
	}
}

// The probability rows must arrive in the order the request declared, so what
// the UI shows lines up with what the user wrote. Ranking is useful, but it
// must not silently reorder the author's own list.
func TestProbabilityRowsKeepTheQuestionOrder(t *testing.T) {
	server := newStubServer(t)

	got := decode[playground.DecideResponse](t, do(t, server, http.MethodPost, "/api/decide", `{
		"state": "a ticket",
		"questions": [{"name": "label", "type": "choice", "instructions": "?", "options_list": [
			{"key": "z", "text": "first"},
			{"key": "m", "text": "second"},
			{"key": "a", "text": "third"}
		]}]
	}`))

	if len(got.Questions) != 1 || len(got.Questions[0].Options) != 3 {
		t.Fatalf("questions = %+v", got.Questions)
	}

	want := []string{"z", "m", "a"}
	for i, option := range got.Questions[0].Options {
		if option.Key != want[i] {
			t.Errorf("option[%d] = %q, want %q: the rows lost the declared order", i, option.Key, want[i])
		}
	}

	rows := got.Answers[0].Probabilities
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	for i, row := range rows {
		if row.Key != want[i] {
			t.Errorf("row[%d] = %q, want %q", i, row.Key, want[i])
		}
	}
}

// The best row is marked, not promoted: the panel highlights it in place.
func TestScoreRowsFollowTheScaleOrder(t *testing.T) {
	server := newStubServer(t)

	got := decode[playground.DecideResponse](t, do(t, server, http.MethodPost, "/api/decide", `{
		"state": "a ticket",
		"questions": [{
			"name": "urgency", "type": "score", "instructions": "how urgent?",
			"scale": ["Whenever", "Soon", "Blocking revenue right now"]
		}]
	}`))

	if len(got.Answers) != 1 {
		t.Fatalf("answers = %d, want 1", len(got.Answers))
	}

	// A scale is an ordered instrument. Ranking the levels by probability
	// puts the strongest one on top and scrambles the rest into 1, 2, 0.
	want := []string{"0", "1", "2"}
	rows := got.Answers[0].Probabilities
	if len(rows) != len(want) {
		t.Fatalf("rows = %d, want %d", len(rows), len(want))
	}
	for i, row := range rows {
		if row.Key != want[i] {
			t.Errorf("row %d = %q, want %q", i, row.Key, want[i])
		}
	}

	best := 0
	for _, row := range rows {
		if row.Best {
			best++
		}
	}
	if best != 1 {
		t.Errorf("%d rows are marked best, want exactly 1", best)
	}
}

func TestProbabilityRowsMarkTheBestInPlace(t *testing.T) {
	server := newStubServer(t)

	got := decode[playground.DecideResponse](t, do(t, server, http.MethodPost, "/api/decide", `{
		"state": "a ticket",
		"questions": [{"name": "label", "type": "choice", "instructions": "?", "options": {"a": "A", "b": "B"}}]
	}`))

	// Declared order a, b, and the stub ties them, so exactly one row must
	// carry the mark even though no row is more probable than another.
	want := []string{"a", "b"}
	rows := got.Answers[0].Probabilities
	if len(rows) != len(want) {
		t.Fatalf("rows = %d, want %d", len(rows), len(want))
	}

	bests := 0
	for i, row := range rows {
		if row.Key != want[i] {
			t.Errorf("row %d = %q, want %q", i, row.Key, want[i])
		}
		if row.Best {
			bests++
		}
	}
	if bests != 1 {
		t.Errorf("%d rows are marked best, want exactly 1", bests)
	}
}

// History is what makes a run reviewable: a stored response must be complete
// enough to replay without sending the request again.
func TestHistoryReplaysAStoredResponse(t *testing.T) {
	server := newStubServer(t)

	body := `{
		"state": "a ticket about billing",
		"questions": [{"name": "label", "type": "choice", "instructions": "?", "options": {"a": "billing", "b": "other"}}]
	}`
	do(t, server, http.MethodPost, "/api/decide", body)

	got := decode[struct {
		Runs []playground.HistoryView `json:"runs"`
	}](t, do(t, server, http.MethodGet, "/api/history", ""))
	if len(got.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(got.Runs))
	}
	run := got.Runs[0]
	if !run.OK {
		t.Error("the run is recorded as failed")
	}
	if !containsAny(run.Questions, "label") {
		t.Errorf("questions = %v, want them to include the question name", run.Questions)
	}
	if run.Response == nil {
		t.Fatal("no stored response to replay")
	}
	if run.Body == nil {
		t.Fatal("no stored request to restore into the editor")
	}
	if run.Summary == "" {
		t.Error("no summary line for the history list")
	}
}

// A failed run has no result to replay, so it must not pretend to have one.
func TestFailedRunStoresNoResponse(t *testing.T) {
	server := newStubServer(t)

	do(t, server, http.MethodPost, "/api/decide", `{
		"state": "a ticket",
		"questions": [{"name": "label", "type": "choice", "instructions": "?"}]
	}`)

	got := decode[struct {
		Runs []playground.HistoryView `json:"runs"`
	}](t, do(t, server, http.MethodGet, "/api/history", ""))
	if len(got.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(got.Runs))
	}
	if got.Runs[0].OK {
		t.Error("an invalid request was recorded as a successful run")
	}
	if got.Runs[0].Response != nil {
		t.Error("a failed run stored a response")
	}
	if got.Runs[0].Error == nil {
		t.Error("a failed run stored no error")
	}
}

// The state limit belongs to the provider, so it is caught before any bytes go
// out rather than as an opaque rejection.
func TestDecideRejectsAnOversizedStateBeforeSending(t *testing.T) {
	server := newStubServer(t)

	resp := do(t, server, http.MethodPost, "/api/decide", `{
		"state": "`+strings.Repeat("x", decide.MaxStateBytes+1)+`",
		"questions": [{"name": "label", "type": "choice", "instructions": "?", "options": {"a": "A", "b": "B"}}]
	}`)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if !containsAny([]string{mustBody(t, resp)}, "state is") {
		t.Error("the error does not name the state as the oversized field")
	}
}
