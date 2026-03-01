// Package gitdiff provides utilities for generating human-readable summaries
// of git changes between Thor deployments.
//
// Edison 🤖 — Implemented as part of the autonomous upgrade loop.
// Feature: Git diff summary — shows what changed in each deploy.
package gitdiff

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DeployMarkerFile is where we store the git commit hash at deploy time.
// safe-deploy.sh writes this file after each successful deploy.
const DeployMarkerFile = "/data/data/com.termux/files/home/.thor/last_deploy_commit"

// ThorRepoPath is the default path to the Thor git repository.
const ThorRepoPath = "/data/data/com.termux/files/home/thor"

// DiffSummary holds the structured result of a git diff between two commits.
type DiffSummary struct {
	FromCommit  string    // Short hash of the "before" commit
	ToCommit    string    // Short hash of the "after" (current) commit
	FromMessage string    // Commit message of the "before" commit
	ToMessage   string    // Commit message of the "after" commit
	DeployedAt  time.Time // When the last deploy happened (from marker file mtime)

	// File-level stats
	FilesChanged int
	Insertions   int
	Deletions    int

	// Changed files grouped by category
	GoFiles      []string // .go source files changed
	ConfigFiles  []string // config/yaml/json files changed
	ScriptFiles  []string // shell scripts changed
	DocFiles     []string // markdown/docs changed
	OtherFiles   []string // anything else

	// Commit messages between the two commits
	CommitLog []string
}

// GetLastDeployCommit reads the stored commit hash from the deploy marker file.
// Returns empty string if the file doesn't exist or can't be read.
func GetLastDeployCommit() string {
	data, err := os.ReadFile(DeployMarkerFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// SaveCurrentCommit writes the current HEAD commit hash to the deploy marker file.
// This should be called by safe-deploy.sh after a successful deploy.
func SaveCurrentCommit(repoPath string) error {
	if repoPath == "" {
		repoPath = ThorRepoPath
	}

	// Get current HEAD short hash
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git rev-parse failed: %w", err)
	}

	hash := strings.TrimSpace(string(out))
	if hash == "" {
		return fmt.Errorf("empty commit hash")
	}

	// Ensure directory exists
	dir := filepath.Dir(DeployMarkerFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	return os.WriteFile(DeployMarkerFile, []byte(hash+"\n"), 0644)
}

// GenerateSummary produces a DiffSummary between the last deployed commit
// and the current HEAD. If no deploy marker exists, it compares HEAD~1 to HEAD.
func GenerateSummary(repoPath string) (*DiffSummary, error) {
	if repoPath == "" {
		repoPath = ThorRepoPath
	}

	// Get current HEAD
	toCommit, err := runGit(repoPath, "rev-parse", "--short", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to get current HEAD: %w", err)
	}

	// Determine the "from" commit
	fromCommit := GetLastDeployCommit()
	if fromCommit == "" || fromCommit == toCommit {
		// Fall back to parent commit
		parent, err := runGit(repoPath, "rev-parse", "--short", "HEAD~1")
		if err != nil {
			// Only one commit in history
			return &DiffSummary{
				FromCommit: "(initial)",
				ToCommit:   toCommit,
				ToMessage:  "Initial commit",
				CommitLog:  []string{"(no previous commit to compare)"},
			}, nil
		}
		fromCommit = parent
	}

	summary := &DiffSummary{
		FromCommit: fromCommit,
		ToCommit:   toCommit,
	}

	// Get deploy timestamp from marker file mtime
	if info, err := os.Stat(DeployMarkerFile); err == nil {
		summary.DeployedAt = info.ModTime()
	}

	// Get commit messages
	summary.FromMessage = getCommitMessage(repoPath, fromCommit)
	summary.ToMessage = getCommitMessage(repoPath, toCommit)

	// Get commit log between the two commits
	logOutput, err := runGit(repoPath, "log", "--oneline", fromCommit+".."+toCommit)
	if err == nil && logOutput != "" {
		lines := strings.Split(strings.TrimSpace(logOutput), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				summary.CommitLog = append(summary.CommitLog, strings.TrimSpace(line))
			}
		}
	}
	if len(summary.CommitLog) == 0 {
		summary.CommitLog = []string{"(no new commits since last deploy)"}
	}

	// Get diff stats
	statOutput, err := runGit(repoPath, "diff", "--stat", fromCommit+".."+toCommit)
	if err == nil && statOutput != "" {
		parseStatOutput(statOutput, summary)
	}

	// Get list of changed files with their types
	nameOutput, err := runGit(repoPath, "diff", "--name-only", fromCommit+".."+toCommit)
	if err == nil && nameOutput != "" {
		categorizeFiles(nameOutput, summary)
	}

	return summary, nil
}

// FormatSummary returns a human-readable string representation of the diff summary.
// This is suitable for sending via Telegram or displaying in the CLI.
func FormatSummary(s *DiffSummary) string {
	if s == nil {
		return "❌ Could not generate diff summary"
	}

	var sb strings.Builder

	sb.WriteString("📦 *Thor Deploy Diff Summary*\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// Commit range
	sb.WriteString(fmt.Sprintf("🔀 *Changes:* `%s` → `%s`\n", s.FromCommit, s.ToCommit))

	if !s.DeployedAt.IsZero() {
		sb.WriteString(fmt.Sprintf("🕐 *Last Deploy:* %s\n", s.DeployedAt.Format("2006-01-02 15:04:05")))
	}

	sb.WriteString("\n")

	// Commit log
	sb.WriteString("📝 *Commits:*\n")
	for _, commit := range s.CommitLog {
		sb.WriteString(fmt.Sprintf("  • %s\n", commit))
	}
	sb.WriteString("\n")

	// Stats
	if s.FilesChanged > 0 {
		sb.WriteString(fmt.Sprintf("📊 *Stats:* %d files changed, +%d/-%d lines\n\n",
			s.FilesChanged, s.Insertions, s.Deletions))
	}

	// Changed files by category
	if len(s.GoFiles) > 0 {
		sb.WriteString(fmt.Sprintf("🔧 *Go Source* (%d files):\n", len(s.GoFiles)))
		for _, f := range s.GoFiles {
			sb.WriteString(fmt.Sprintf("  `%s`\n", f))
		}
		sb.WriteString("\n")
	}

	if len(s.ScriptFiles) > 0 {
		sb.WriteString(fmt.Sprintf("📜 *Scripts* (%d files):\n", len(s.ScriptFiles)))
		for _, f := range s.ScriptFiles {
			sb.WriteString(fmt.Sprintf("  `%s`\n", f))
		}
		sb.WriteString("\n")
	}

	if len(s.ConfigFiles) > 0 {
		sb.WriteString(fmt.Sprintf("⚙️  *Config* (%d files):\n", len(s.ConfigFiles)))
		for _, f := range s.ConfigFiles {
			sb.WriteString(fmt.Sprintf("  `%s`\n", f))
		}
		sb.WriteString("\n")
	}

	if len(s.DocFiles) > 0 {
		sb.WriteString(fmt.Sprintf("📚 *Docs* (%d files):\n", len(s.DocFiles)))
		for _, f := range s.DocFiles {
			sb.WriteString(fmt.Sprintf("  `%s`\n", f))
		}
		sb.WriteString("\n")
	}

	if len(s.OtherFiles) > 0 {
		sb.WriteString(fmt.Sprintf("📁 *Other* (%d files):\n", len(s.OtherFiles)))
		for _, f := range s.OtherFiles {
			sb.WriteString(fmt.Sprintf("  `%s`\n", f))
		}
		sb.WriteString("\n")
	}

	// If nothing changed
	total := len(s.GoFiles) + len(s.ScriptFiles) + len(s.ConfigFiles) + len(s.DocFiles) + len(s.OtherFiles)
	if total == 0 && s.FilesChanged == 0 {
		sb.WriteString("✨ No file changes detected since last deploy.\n")
	}

	return sb.String()
}

// FormatSummaryPlain returns a plain-text version without markdown formatting.
// Useful for CLI output.
func FormatSummaryPlain(s *DiffSummary) string {
	if s == nil {
		return "Could not generate diff summary"
	}

	var sb strings.Builder

	sb.WriteString("=== Thor Deploy Diff Summary ===\n\n")
	sb.WriteString(fmt.Sprintf("Changes: %s → %s\n", s.FromCommit, s.ToCommit))

	if !s.DeployedAt.IsZero() {
		sb.WriteString(fmt.Sprintf("Last Deploy: %s\n", s.DeployedAt.Format("2006-01-02 15:04:05")))
	}

	sb.WriteString("\nCommits:\n")
	for _, commit := range s.CommitLog {
		sb.WriteString(fmt.Sprintf("  • %s\n", commit))
	}

	if s.FilesChanged > 0 {
		sb.WriteString(fmt.Sprintf("\nStats: %d files changed, +%d/-%d lines\n",
			s.FilesChanged, s.Insertions, s.Deletions))
	}

	total := len(s.GoFiles) + len(s.ScriptFiles) + len(s.ConfigFiles) + len(s.DocFiles) + len(s.OtherFiles)
	if total > 0 {
		sb.WriteString("\nChanged files:\n")
		allFiles := append(append(append(append(s.GoFiles, s.ScriptFiles...), s.ConfigFiles...), s.DocFiles...), s.OtherFiles...)
		for _, f := range allFiles {
			sb.WriteString(fmt.Sprintf("  %s\n", f))
		}
	}

	return sb.String()
}

// --- Internal helpers ---

// runGit executes a git command in the given repo directory and returns stdout.
func runGit(repoPath string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// getCommitMessage returns the first line of a commit message.
func getCommitMessage(repoPath, commit string) string {
	msg, err := runGit(repoPath, "log", "--format=%s", "-1", commit)
	if err != nil {
		return "(unknown)"
	}
	return msg
}

// parseStatOutput parses the output of `git diff --stat` to extract counts.
// Example last line: " 4 files changed, 478 insertions(+), 5 deletions(-)"
func parseStatOutput(output string, s *DiffSummary) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "file") && strings.Contains(line, "changed") {
			// Parse: "N files changed, M insertions(+), K deletions(-)"
			var files, ins, del int
			// Try various formats
			fmt.Sscanf(line, "%d file", &files)
			s.FilesChanged = files

			// Find insertions
			if idx := strings.Index(line, "insertion"); idx > 0 {
				part := line[:idx]
				// Find last number before "insertion"
				parts := strings.Fields(part)
				if len(parts) > 0 {
					fmt.Sscanf(parts[len(parts)-1], "%d", &ins)
					s.Insertions = ins
				}
			}

			// Find deletions
			if idx := strings.Index(line, "deletion"); idx > 0 {
				part := line[:idx]
				parts := strings.Fields(part)
				if len(parts) > 0 {
					fmt.Sscanf(parts[len(parts)-1], "%d", &del)
					s.Deletions = del
				}
			}
			break
		}
	}
}

// categorizeFiles sorts changed file paths into categories.
func categorizeFiles(nameOutput string, s *DiffSummary) {
	files := strings.Split(strings.TrimSpace(nameOutput), "\n")
	for _, f := range files {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}

		ext := strings.ToLower(filepath.Ext(f))
		base := strings.ToLower(filepath.Base(f))

		switch {
		case ext == ".go":
			s.GoFiles = append(s.GoFiles, f)
		case ext == ".sh" || strings.HasSuffix(base, ".bash"):
			s.ScriptFiles = append(s.ScriptFiles, f)
		case ext == ".yml" || ext == ".yaml" || ext == ".json" || ext == ".toml" || base == ".env":
			s.ConfigFiles = append(s.ConfigFiles, f)
		case ext == ".md" || ext == ".txt" || ext == ".rst":
			s.DocFiles = append(s.DocFiles, f)
		default:
			s.OtherFiles = append(s.OtherFiles, f)
		}
	}
}
