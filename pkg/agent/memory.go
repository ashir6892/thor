// Thor - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Thor contributors

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"thor/pkg/fileutil"
)

// MemoryStore manages persistent memory for the agent.
// - Long-term memory: memory/MEMORY.md
// - Recent conversation log: memory/RECENT.md (rolling last N exchanges)
// - Daily notes: memory/YYYYMM/YYYYMMDD.md
type MemoryStore struct {
	workspace   string
	memoryDir   string
	memoryFile  string
	recentFile  string
}

// NewMemoryStore creates a new MemoryStore with the given workspace path.
// It ensures the memory directory exists.
func NewMemoryStore(workspace string) *MemoryStore {
	memoryDir := filepath.Join(workspace, "memory")
	memoryFile := filepath.Join(memoryDir, "MEMORY.md")
	recentFile := filepath.Join(memoryDir, "RECENT.md")

	// Ensure memory directory exists
	os.MkdirAll(memoryDir, 0o755)

	return &MemoryStore{
		workspace:  workspace,
		memoryDir:  memoryDir,
		memoryFile: memoryFile,
		recentFile: recentFile,
	}
}

// getTodayFile returns the path to today's daily note file (memory/YYYYMM/YYYYMMDD.md).
func (ms *MemoryStore) getTodayFile() string {
	today := time.Now().Format("20060102") // YYYYMMDD
	monthDir := today[:6]                  // YYYYMM
	filePath := filepath.Join(ms.memoryDir, monthDir, today+".md")
	return filePath
}

// ReadLongTerm reads the long-term memory (MEMORY.md).
// Returns empty string if the file doesn't exist.
func (ms *MemoryStore) ReadLongTerm() string {
	if data, err := os.ReadFile(ms.memoryFile); err == nil {
		return string(data)
	}
	return ""
}

// WriteLongTerm writes content to the long-term memory file (MEMORY.md).
func (ms *MemoryStore) WriteLongTerm(content string) error {
	// Use unified atomic write utility with explicit sync for flash storage reliability.
	// Using 0o600 (owner read/write only) for secure default permissions.
	return fileutil.WriteFileAtomic(ms.memoryFile, []byte(content), 0o600)
}

// ReadToday reads today's daily note.
// Returns empty string if the file doesn't exist.
func (ms *MemoryStore) ReadToday() string {
	todayFile := ms.getTodayFile()
	if data, err := os.ReadFile(todayFile); err == nil {
		return string(data)
	}
	return ""
}

// AppendToday appends content to today's daily note.
// If the file doesn't exist, it creates a new file with a date header.
func (ms *MemoryStore) AppendToday(content string) error {
	todayFile := ms.getTodayFile()

	// Ensure month directory exists
	monthDir := filepath.Dir(todayFile)
	if err := os.MkdirAll(monthDir, 0o755); err != nil {
		return err
	}

	var existingContent string
	if data, err := os.ReadFile(todayFile); err == nil {
		existingContent = string(data)
	}

	var newContent string
	if existingContent == "" {
		// Add header for new day
		header := fmt.Sprintf("# %s\n\n", time.Now().Format("2006-01-02"))
		newContent = header + content
	} else {
		// Append to existing content
		newContent = existingContent + "\n" + content
	}

	// Use unified atomic write utility with explicit sync for flash storage reliability.
	return fileutil.WriteFileAtomic(todayFile, []byte(newContent), 0o600)
}

// GetRecentDailyNotes returns daily notes from the last N days.
// Contents are joined with "---" separator.
func (ms *MemoryStore) GetRecentDailyNotes(days int) string {
	var sb strings.Builder
	first := true

	for i := 0; i < days; i++ {
		date := time.Now().AddDate(0, 0, -i)
		dateStr := date.Format("20060102") // YYYYMMDD
		monthDir := dateStr[:6]            // YYYYMM
		filePath := filepath.Join(ms.memoryDir, monthDir, dateStr+".md")

		if data, err := os.ReadFile(filePath); err == nil {
			if !first {
				sb.WriteString("\n\n---\n\n")
			}
			sb.Write(data)
			first = false
		}
	}

	return sb.String()
}

// ReadRecent reads the rolling recent conversation log (RECENT.md).
// Returns empty string if the file doesn't exist.
func (ms *MemoryStore) ReadRecent() string {
	if data, err := os.ReadFile(ms.recentFile); err == nil {
		return string(data)
	}
	return ""
}

// recentEntry represents a single logged exchange in RECENT.md.
type recentEntry struct {
	timestamp string
	user      string
	assistant string
}

// parseRecentEntries parses the RECENT.md content into structured entries.
// Each entry is delimited by "---".
func parseRecentEntries(content string) []recentEntry {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	// Strip optional header line "# Recent Conversation Log\n\n"
	content = strings.TrimPrefix(content, "# Recent Conversation Log\n\n")

	blocks := strings.Split(content, "\n---\n")
	entries := make([]recentEntry, 0, len(blocks))
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		var e recentEntry
		for _, line := range strings.Split(block, "\n") {
			if ts, ok := strings.CutPrefix(line, "**Time:** "); ok {
				e.timestamp = ts
			} else if u, ok := strings.CutPrefix(line, "**User:** "); ok {
				e.user = u
			} else if a, ok := strings.CutPrefix(line, "**Assistant:** "); ok {
				e.assistant = a
			}
		}
		if e.user != "" || e.assistant != "" {
			entries = append(entries, e)
		}
	}
	return entries
}

// formatRecentEntries serialises entries back to RECENT.md content.
func formatRecentEntries(entries []recentEntry) string {
	var sb strings.Builder
	sb.WriteString("# Recent Conversation Log\n\n")
	for i, e := range entries {
		if i > 0 {
			sb.WriteString("\n---\n")
		}
		fmt.Fprintf(&sb, "**Time:** %s\n**User:** %s\n**Assistant:** %s\n", e.timestamp, e.user, e.assistant)
	}
	return sb.String()
}

// AppendRecent appends a new user/assistant exchange to RECENT.md,
// keeping only the last maxRecentEntries entries (rolling window).
func (ms *MemoryStore) AppendRecent(userMsg, assistantMsg string, maxEntries int) error {
	if maxEntries <= 0 {
		maxEntries = 10
	}

	existing := ms.ReadRecent()
	entries := parseRecentEntries(existing)

	// Truncate long messages to keep RECENT.md compact.
	const maxLen = 300
	if len(userMsg) > maxLen {
		userMsg = userMsg[:maxLen] + "…"
	}
	if len(assistantMsg) > maxLen {
		assistantMsg = assistantMsg[:maxLen] + "…"
	}

	entries = append(entries, recentEntry{
		timestamp: time.Now().Format("2006-01-02 15:04"),
		user:      userMsg,
		assistant: assistantMsg,
	})

	// Keep only the last N entries
	if len(entries) > maxEntries {
		entries = entries[len(entries)-maxEntries:]
	}

	content := formatRecentEntries(entries)
	return fileutil.WriteFileAtomic(ms.recentFile, []byte(content), 0o600)
}

// GetMemoryContext returns formatted memory context for the agent prompt.
// Includes long-term memory, recent conversation log, and recent daily notes.
func (ms *MemoryStore) GetMemoryContext() string {
	longTerm := ms.ReadLongTerm()
	recent := ms.ReadRecent()
	recentNotes := ms.GetRecentDailyNotes(3)

	if longTerm == "" && recent == "" && recentNotes == "" {
		return ""
	}

	var sb strings.Builder

	if longTerm != "" {
		sb.WriteString("## Long-term Memory\n\n")
		sb.WriteString(longTerm)
	}

	if recent != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n\n---\n\n")
		}
		sb.WriteString("## Recent Conversation Log\n\n")
		sb.WriteString(recent)
	}

	if recentNotes != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n\n---\n\n")
		}
		sb.WriteString("## Recent Daily Notes\n\n")
		sb.WriteString(recentNotes)
	}

	return sb.String()
}
