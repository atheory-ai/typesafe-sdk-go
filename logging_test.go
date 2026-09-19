package typesafe

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type logEntry struct{ level, message string }
type memoryLogger struct {
	mu      sync.Mutex
	entries []logEntry
}

func (l *memoryLogger) add(level, message string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, logEntry{level, fmt.Sprintf(message, args...)})
}
func (l *memoryLogger) Debug(m string, a ...any) { l.add("debug", m, a...) }
func (l *memoryLogger) Info(m string, a ...any)  { l.add("info", m, a...) }
func (l *memoryLogger) Warn(m string, a ...any)  { l.add("warn", m, a...) }
func (l *memoryLogger) Error(m string, a ...any) { l.add("error", m, a...) }

func TestLogLevelAndCredentialRedaction(t *testing.T) {
	sink := &memoryLogger{}
	client := newTestClient(t, func(*http.Request) (*http.Response, error) { return response(200, `{"models":[]}`), nil }, WithLogger(sink), WithLogLevel(LogDebug))
	if _, err := client.Models.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, entry := range sink.entries {
		joined += entry.level + ":" + entry.message + "\n"
	}
	if strings.Contains(joined, "secret") || !strings.Contains(joined, "Bearer ***") || !strings.Contains(joined, "response body") {
		t.Fatalf("logs:\n%s", joined)
	}
}

func TestInfoDropsDebug(t *testing.T) {
	sink := &memoryLogger{}
	client := newTestClient(t, func(*http.Request) (*http.Response, error) { return response(200, `{"models":[]}`), nil }, WithLogger(sink), WithLogLevel(LogInfo))
	if _, err := client.Models.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sink.entries) != 1 || sink.entries[0].level != "info" {
		t.Fatalf("entries: %#v", sink.entries)
	}
}

func TestRedactHeaders(t *testing.T) {
	input := map[string]string{"Authorization": "Bearer very-secret-key", "x-api-key": "short", "Cookie": "session", "X-Trace": "ok"}
	got := RedactHeaders(input)
	if got["Authorization"] != "Bearer ***-key" || got["x-api-key"] != "***" || got["Cookie"] != "***" || got["X-Trace"] != "ok" {
		t.Fatalf("redacted: %#v", got)
	}
	if input["Cookie"] != "session" {
		t.Fatal("mutated input")
	}
}

func TestLoggerLevelMethods(t *testing.T) {
	sink := &memoryLogger{}
	logger := levelLogger{sink: sink, level: LogWarn}
	logger.Debug("debug")
	logger.Info("info")
	logger.Warn("warn")
	logger.Error("error")
	if len(sink.entries) != 2 || sink.entries[0].level != "warn" || sink.entries[1].level != "error" {
		t.Fatalf("entries: %#v", sink.entries)
	}
	// Smoke test the default sink's four standard methods.
	var defaults Logger = defaultLogger{}
	defaults.Debug("debug")
	defaults.Info("info")
	defaults.Warn("warn")
	defaults.Error("error")
}
