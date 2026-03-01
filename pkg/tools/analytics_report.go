// analytics_report.go — Tool Usage Analytics Report Generator
// Edison 🤖: Built as part of Brain Loop Cycle 2 (Tool Analytics + Auto-Optimization)
// Reads ~/.thor/metrics/tool_analytics.jsonl and produces ranked reports.
package tools

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ToolStats holds aggregated statistics for a single tool.
type ToolStats struct {
	Name      string
	Calls     int
	Errors    int
	TotalMs   int64
	AvgMs     float64
	ErrorRate float64
	P95Ms     int64 // 95th percentile latency
	LastSeen  string
}

// AnalyticsReport holds the full report for all tools.
type AnalyticsReport struct {
	GeneratedAt string
	Period      string // e.g. "last 7 days"
	Tools       []ToolStats
	Suggestions []string
}

// GenerateReport reads tool_analytics.jsonl and produces a ranked AnalyticsReport.
// sinceHours: only include entries from the last N hours (0 = all time)
func GenerateReport(sinceHours int) (*AnalyticsReport, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	logPath := filepath.Join(home, ".thor", "metrics", "tool_analytics.jsonl")

	f, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("no analytics data yet: %w", err)
	}
	defer f.Close()

	cutoff := time.Time{}
	if sinceHours > 0 {
		cutoff = time.Now().UTC().Add(-time.Duration(sinceHours) * time.Hour)
	}

	// Aggregate per-tool stats using an anonymous struct map
	type rawStats struct {
		calls     int
		errors    int
		totalMs   int64
		latencies []int64
		lastSeen  time.Time
	}
	statsMap := make(map[string]*rawStats)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry toolAnalyticsEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		ts, err := time.Parse(time.RFC3339, entry.Timestamp)
		if err != nil {
			continue
		}
		if !cutoff.IsZero() && ts.Before(cutoff) {
			continue
		}

		s, ok := statsMap[entry.Tool]
		if !ok {
			s = &rawStats{}
			statsMap[entry.Tool] = s
		}
		s.calls++
		s.totalMs += entry.DurationMs
		s.latencies = append(s.latencies, entry.DurationMs)
		if entry.IsError {
			s.errors++
		}
		if ts.After(s.lastSeen) {
			s.lastSeen = ts
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading analytics file: %w", err)
	}

	// Build sorted ToolStats slice (by call count desc)
	toolStats := make([]ToolStats, 0, len(statsMap))
	for name, s := range statsMap {
		avg := float64(0)
		if s.calls > 0 {
			avg = float64(s.totalMs) / float64(s.calls)
		}
		errRate := float64(0)
		if s.calls > 0 {
			errRate = float64(s.errors) / float64(s.calls) * 100
		}

		// Calculate P95
		sorted := make([]int64, len(s.latencies))
		copy(sorted, s.latencies)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		p95idx := int(float64(len(sorted)) * 0.95)
		if p95idx >= len(sorted) {
			p95idx = len(sorted) - 1
		}
		p95 := int64(0)
		if len(sorted) > 0 {
			p95 = sorted[p95idx]
		}

		lastSeenStr := ""
		if !s.lastSeen.IsZero() {
			lastSeenStr = s.lastSeen.Format("2006-01-02 15:04")
		}

		toolStats = append(toolStats, ToolStats{
			Name:      name,
			Calls:     s.calls,
			Errors:    s.errors,
			TotalMs:   s.totalMs,
			AvgMs:     avg,
			ErrorRate: errRate,
			P95Ms:     p95,
			LastSeen:  lastSeenStr,
		})
	}
	sort.Slice(toolStats, func(i, j int) bool {
		return toolStats[i].Calls > toolStats[j].Calls
	})

	// Generate suggestions
	suggestions := generateSuggestions(toolStats)

	period := "all time"
	if sinceHours > 0 {
		if sinceHours == 168 {
			period = "last 7 days"
		} else {
			period = fmt.Sprintf("last %dh", sinceHours)
		}
	}

	return &AnalyticsReport{
		GeneratedAt: time.Now().UTC().Format("2006-01-02 15:04 UTC"),
		Period:      period,
		Tools:       toolStats,
		Suggestions: suggestions,
	}, nil
}

// generateSuggestions produces actionable recommendations from tool stats.
func generateSuggestions(stats []ToolStats) []string {
	var suggestions []string
	for _, s := range stats {
		if s.ErrorRate > 25 && s.Calls >= 5 {
			suggestions = append(suggestions, fmt.Sprintf(
				"⚠️ `%s` has %.0f%% error rate (%d/%d calls failed) — investigate or disable",
				s.Name, s.ErrorRate, s.Errors, s.Calls,
			))
		}
		if s.AvgMs > 5000 && s.Calls >= 3 {
			suggestions = append(suggestions, fmt.Sprintf(
				"🐢 `%s` is slow (avg %.1fs) — consider caching or parallelizing",
				s.Name, s.AvgMs/1000,
			))
		}
	}
	if len(suggestions) == 0 {
		suggestions = append(suggestions, "✅ All tools performing within normal parameters")
	}
	return suggestions
}

// FormatReport formats an AnalyticsReport as a Telegram-friendly markdown string.
func FormatReport(r *AnalyticsReport) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 *Tool Analytics Report* (%s)\n", r.Period))
	sb.WriteString(fmt.Sprintf("_Generated: %s_\n\n", r.GeneratedAt))

	if len(r.Tools) == 0 {
		sb.WriteString("No tool calls recorded yet.\n")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("*Total tools tracked: %d*\n\n", len(r.Tools)))
	sb.WriteString("*Top Tools by Usage:*\n")

	limit := 10
	if len(r.Tools) < limit {
		limit = len(r.Tools)
	}
	for i, t := range r.Tools[:limit] {
		errStr := ""
		if t.Errors > 0 {
			errStr = fmt.Sprintf(" ⚠️%.0f%%err", t.ErrorRate)
		}
		latStr := ""
		if t.AvgMs > 0 {
			latStr = fmt.Sprintf(", avg %.0fms", t.AvgMs)
		}
		sb.WriteString(fmt.Sprintf("%d. `%s` — %d calls%s%s\n",
			i+1, t.Name, t.Calls, latStr, errStr))
	}

	sb.WriteString("\n*Suggestions:*\n")
	for _, s := range r.Suggestions {
		sb.WriteString(s + "\n")
	}
	return sb.String()
}
