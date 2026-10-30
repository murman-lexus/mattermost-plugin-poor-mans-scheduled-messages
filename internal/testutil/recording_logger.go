package testutil

import (
	"fmt"
	"strings"
	"sync"
)

// RecordingLogger records all levels, formatting errors as a real log sink would.
type RecordingLogger struct {
	mu     sync.Mutex
	output strings.Builder
}

// Debug records a debug event.
func (l *RecordingLogger) Debug(msg string, fields ...any) { l.record("DEBUG", msg, fields) }

// Info records an informational event.
func (l *RecordingLogger) Info(msg string, fields ...any) { l.record("INFO", msg, fields) }

// Warn records a warning event.
func (l *RecordingLogger) Warn(msg string, fields ...any) { l.record("WARN", msg, fields) }

// Error records an error event.
func (l *RecordingLogger) Error(msg string, fields ...any) { l.record("ERROR", msg, fields) }

func (l *RecordingLogger) record(level, msg string, fields []any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = fmt.Fprintf(&l.output, "%s %s %v\n", level, msg, fields)
}

// String returns the recorded events.
func (l *RecordingLogger) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.output.String()
}
