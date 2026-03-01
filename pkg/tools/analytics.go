// Thor Tool Analytics — tracks per-tool call counts, latency, and error rates.
// Data is appended to ~/.thor/metrics/tool_analytics.jsonl (one JSON object per line).
// This runs asynchronously and never blocks tool execution.
package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// toolAnalyticsEntry is a single tool call record written to JSONL.
type toolAnalyticsEntry struct {
	Timestamp  string  `json:"ts"`
	Tool       string  `json:"tool"`
	DurationMs int64   `json:"duration_ms"`
	IsError    bool    `json:"error"`
	ResultLen  int     `json:"result_len"`
}

// analyticsWriter is a singleton background writer for tool analytics.
type analyticsWriter struct {
	mu      sync.Mutex
	logPath string
	once    sync.Once
}

var globalAnalytics = &analyticsWriter{}

// initPath sets up the analytics log path once.
func (a *analyticsWriter) initPath() {
	a.once.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			home = os.Getenv("HOME")
		}
		dir := filepath.Join(home, ".thor", "metrics")
		_ = os.MkdirAll(dir, 0o755)
		a.logPath = filepath.Join(dir, "tool_analytics.jsonl")
	})
}

// record appends a tool analytics entry asynchronously.
func (a *analyticsWriter) record(tool string, duration time.Duration, isError bool, resultLen int) {
	a.initPath()
	entry := toolAnalyticsEntry{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Tool:       tool,
		DurationMs: duration.Milliseconds(),
		IsError:    isError,
		ResultLen:  resultLen,
	}
	go func() {
		data, err := json.Marshal(entry)
		if err != nil {
			return
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		f, err := os.OpenFile(a.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = f.Write(append(data, '\n'))
	}()
}

// RecordToolAnalytics records a tool call's performance metrics.
// Called by the ToolRegistry after each tool execution.
func RecordToolAnalytics(tool string, duration time.Duration, isError bool, resultLen int) {
	globalAnalytics.record(tool, duration, isError, resultLen)
}
