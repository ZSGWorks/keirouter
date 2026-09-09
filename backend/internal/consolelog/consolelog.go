// Package consolelog provides a thread-safe log buffer that captures request
// pipeline output and streams it to dashboard clients via SSE.
//
// Each record is a structured Entry carrying a short, human-readable message
// plus an optional Detail block. The dashboard renders the message inline and
// reveals the detail on demand (click to expand), keeping the live feed easy to
// scan while still exposing the underlying technical context (raw errors, IDs,
// timings) when a developer needs it.
package consolelog

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const maxLines = 500

// Entry is a single structured console log record.
type Entry struct {
	// Seq is a monotonic sequence number, unique per buffer. The dashboard uses
	// it as a stable React key and to track per-row expansion state across
	// re-renders and ring-buffer trimming.
	Seq uint64 `json:"seq"`
	// Time is the formatted wall-clock time (HH:MM:SS.mmm).
	Time string `json:"time"`
	// Level is one of DEBUG, INFO, WARN, ERROR, LOG.
	Level string `json:"level"`
	// Message is the short, human-readable summary shown inline.
	Message string `json:"msg"`
	// Detail is optional technical context revealed when the row is expanded
	// (raw error text, identifiers, byte counts, etc.). Empty when there is
	// nothing extra to show.
	Detail string `json:"detail,omitempty"`
}

// Event represents a log update. If Clear is true, the client should clear its
// buffer; otherwise Entry holds the appended record.
type Event struct {
	Entry Entry
	Clear bool
}

// Listener receives log events via a buffered channel. The channel-based
// approach ensures that slow listeners never block the publisher.
type Listener struct {
	C chan Event
}

// NewListener creates a listener with a buffered event channel.
func NewListener(bufSize int) *Listener {
	if bufSize <= 0 {
		bufSize = 256 // Log streams can be chatty
	}
	return &Listener{C: make(chan Event, bufSize)}
}

// Buffer is a ring buffer of log entries with pub/sub for SSE streaming.
type Buffer struct {
	mu sync.RWMutex
	// minLevelRank filters entries below the configured verbosity: the
	// dashboard streams every entry, so an unfiltered DEBUG gate would
	// format, ring-buffer, and fan out noise on every request. Rank is
	// compared in add(); 0 = keep everything (debug).
	minLevelRank int
	entries      []Entry
	listeners    map[*Listener]struct{}
	seq          uint64 // guarded by mu; monotonic entry counter
}

// levelRank maps log level names to a severity rank. Unknown levels keep
// everything (rank 0). DEBUG < LOG < INFO < WARN < ERROR.
var levelRank = map[string]int{
	"DEBUG": 1,
	"LOG":   2,
	"INFO":  2,
	"WARN":  3,
	"ERROR": 4,
}

// minLevelRankFor converts a config level (debug, info, warn, error) into the
// minimum severity rank a console entry must reach to be buffered.
func minLevelRankFor(level string) int {
	switch strings.ToLower(level) {
	case "debug":
		return 0
	case "warn":
		return levelRank["WARN"]
	case "error":
		return levelRank["ERROR"]
	default:
		return levelRank["LOG"]
	}
}

// SetMinLevel drops entries below the given config level (debug/info/warn/
// error). Safe for concurrent use; intended for one call at startup.
func (b *Buffer) SetMinLevel(level string) {
	b.mu.Lock()
	b.minLevelRank = minLevelRankFor(level)
	b.mu.Unlock()
}

// New creates an empty log buffer.
func New() *Buffer {
	return &Buffer{
		entries:   make([]Entry, 0, 128),
		listeners: make(map[*Listener]struct{}),
	}
}

// Entries returns a snapshot of all buffered log entries.
func (b *Buffer) Entries() []Entry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]Entry, len(b.entries))
	copy(out, b.entries)
	return out
}

// add appends an entry and notifies all listeners.
func (b *Buffer) add(level, message, detail string) {
	b.mu.Lock()
	if levelRank[level] < b.minLevelRank {
		b.mu.Unlock()
		return
	}
	b.seq++
	e := Entry{
		Seq:     b.seq,
		Time:    time.Now().Format("15:04:05.000"),
		Level:   level,
		Message: message,
		Detail:  detail,
	}
	b.entries = append(b.entries, e)
	if len(b.entries) > maxLines {
		// Use copy instead of sub-slicing to release the old backing array for GC.
		n := copy(b.entries, b.entries[len(b.entries)-maxLines:])
		b.entries = b.entries[:n]
	}
	b.mu.Unlock()

	b.publish(Event{Entry: e})
}

// Clear resets the buffer and notifies all listeners.
func (b *Buffer) Clear() {
	b.mu.Lock()
	b.entries = b.entries[:0]
	b.mu.Unlock()

	b.publish(Event{Clear: true})
}

func (b *Buffer) publish(ev Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for l := range b.listeners {
		select {
		case l.C <- ev:
		default:
			// Drop event for slow listener — non-blocking.
		}
	}
}

// Subscribe registers a listener. Call Unsubscribe when done.
func (b *Buffer) Subscribe(l *Listener) {
	b.mu.Lock()
	b.listeners[l] = struct{}{}
	b.mu.Unlock()
}

// Unsubscribe removes a listener.
func (b *Buffer) Unsubscribe(l *Listener) {
	b.mu.Lock()
	delete(b.listeners, l)
	b.mu.Unlock()
}

// Log appends a human-readable message at the given level with an optional
// detail block. Pass an empty detail when there is nothing extra to surface.
// Levels: LOG, INFO, WARN, ERROR, DEBUG.
func (b *Buffer) Log(level, message, detail string) {
	b.add(level, message, detail)
}

// Logf appends a formatted, message-only log line (no detail block). Retained
// for call sites that only need a one-liner.
func (b *Buffer) Logf(level, format string, args ...any) {
	b.add(level, fmt.Sprintf(format, args...), "")
}
