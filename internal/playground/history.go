package playground

import (
	"encoding/json"
	"sync"
	"time"
)

// historyLimit is the number of past runs kept in memory. The playground is a
// local testing tool, so the history is deliberately small and deliberately
// never written to disk.
const historyLimit = 50

// historyEntry is one stored run.
type historyEntry struct {
	id        string
	at        time.Time
	provider  string
	model     string
	ok        bool
	duration  time.Duration
	body      json.RawMessage
	response  json.RawMessage
	err       *ErrorView
	questions []string
	summary   string
}

// historyStore is a fixed-size in-memory ring of past runs. It is safe for
// concurrent use, which matters because decisions are served concurrently.
type historyStore struct {
	mu      sync.RWMutex
	entries []historyEntry
	next    int
}

func newHistoryStore(limit int) *historyStore {
	if limit <= 0 {
		limit = historyLimit
	}
	return &historyStore{entries: make([]historyEntry, limit)}
}

// add stores a run, evicting the oldest once the buffer is full.
func (h *historyStore) add(entry historyEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()

	entry.at = time.Now()
	if entry.id == "" {
		entry.id = newRunID()
	}

	if len(h.entries) < cap(h.entries) {
		h.entries = append(h.entries, entry)
		return
	}

	h.entries[h.next] = entry
	h.next = (h.next + 1) % len(h.entries)
}

// list returns the runs newest first.
func (h *historyStore) list() []HistoryView {
	h.mu.RLock()
	defer h.mu.RUnlock()

	out := make([]HistoryView, 0, len(h.entries))
	for i := range h.entries {
		if h.entries[i].id == "" {
			continue
		}
		out = append(out, toHistoryView(h.entries[i]))
	}

	// The ring buffer is not necessarily in order once it wraps, so sort
	// newest first.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// clear empties the history.
func (h *historyStore) clear() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.entries = make([]historyEntry, len(h.entries))
	h.next = 0
}

func toHistoryView(entry historyEntry) HistoryView {
	view := HistoryView{
		RequestID:  entry.id,
		At:         entry.at.UnixMilli(),
		Provider:   entry.provider,
		Model:      entry.model,
		OK:         entry.ok,
		DurationMS: entry.duration.Milliseconds(),
		Body:       entry.body,
		Response:   entry.response,
		Error:      entry.err,
		Questions:  entry.questions,
		Summary:    entry.summary,
	}
	return view
}

// runIDCounter makes ids unique within one process. The history never leaves
// memory, so a process-local counter is enough.
var runIDCounter struct {
	sync.Mutex
	n uint64
}

func newRunID() string {
	runIDCounter.Lock()
	runIDCounter.n++
	n := runIDCounter.n
	runIDCounter.Unlock()
	return time.Now().UTC().Format("150405") + "-" + strconvUint(n)
}

func strconvUint(n uint64) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	return string(buf[i:])
}
