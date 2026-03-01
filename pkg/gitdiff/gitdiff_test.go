package gitdiff

import (
	"strings"
	"testing"
	"time"
)

func TestFormatSummary_NilSummary(t *testing.T) {
	result := FormatSummary(nil)
	if !strings.Contains(result, "Could not generate") {
		t.Errorf("expected error message for nil summary, got: %s", result)
	}
}

func TestFormatSummaryPlain_NilSummary(t *testing.T) {
	result := FormatSummaryPlain(nil)
	if !strings.Contains(result, "Could not generate") {
		t.Errorf("expected error message for nil summary, got: %s", result)
	}
}

func TestFormatSummary_BasicSummary(t *testing.T) {
	s := &DiffSummary{
		FromCommit:   "abc1234",
		ToCommit:     "def5678",
		FromMessage:  "feat: old feature",
		ToMessage:    "feat: new feature",
		DeployedAt:   time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		FilesChanged: 3,
		Insertions:   42,
		Deletions:    7,
		GoFiles:      []string{"pkg/agent/loop.go", "pkg/gitdiff/gitdiff.go"},
		ScriptFiles:  []string{"scripts/safe-deploy.sh"},
		CommitLog:    []string{"def5678 feat: git diff summary"},
	}

	result := FormatSummary(s)

	// Should contain key elements
	if !strings.Contains(result, "abc1234") {
		t.Error("expected from commit in output")
	}
	if !strings.Contains(result, "def5678") {
		t.Error("expected to commit in output")
	}
	if !strings.Contains(result, "feat: git diff summary") {
		t.Error("expected commit log in output")
	}
	if !strings.Contains(result, "3 files changed") {
		t.Error("expected file count in output")
	}
	if !strings.Contains(result, "pkg/agent/loop.go") {
		t.Error("expected go file in output")
	}
	if !strings.Contains(result, "scripts/safe-deploy.sh") {
		t.Error("expected script file in output")
	}
}

func TestFormatSummaryPlain_BasicSummary(t *testing.T) {
	s := &DiffSummary{
		FromCommit:   "abc1234",
		ToCommit:     "def5678",
		FilesChanged: 2,
		Insertions:   10,
		Deletions:    3,
		GoFiles:      []string{"main.go"},
		CommitLog:    []string{"def5678 fix: something"},
	}

	result := FormatSummaryPlain(s)

	if !strings.Contains(result, "abc1234") {
		t.Error("expected from commit")
	}
	if !strings.Contains(result, "def5678") {
		t.Error("expected to commit")
	}
	if !strings.Contains(result, "main.go") {
		t.Error("expected file in output")
	}
}

func TestFormatSummary_NoChanges(t *testing.T) {
	s := &DiffSummary{
		FromCommit: "abc1234",
		ToCommit:   "abc1234",
		CommitLog:  []string{"(no new commits since last deploy)"},
	}

	result := FormatSummary(s)

	if !strings.Contains(result, "No file changes") {
		t.Errorf("expected 'no changes' message, got: %s", result)
	}
}

func TestCategorizeFiles(t *testing.T) {
	s := &DiffSummary{}
	input := "pkg/agent/loop.go\nscripts/deploy.sh\nconfig.yml\nREADME.md\nassets/logo.png"
	categorizeFiles(input, s)

	if len(s.GoFiles) != 1 || s.GoFiles[0] != "pkg/agent/loop.go" {
		t.Errorf("expected 1 go file, got: %v", s.GoFiles)
	}
	if len(s.ScriptFiles) != 1 || s.ScriptFiles[0] != "scripts/deploy.sh" {
		t.Errorf("expected 1 script file, got: %v", s.ScriptFiles)
	}
	if len(s.ConfigFiles) != 1 || s.ConfigFiles[0] != "config.yml" {
		t.Errorf("expected 1 config file, got: %v", s.ConfigFiles)
	}
	if len(s.DocFiles) != 1 || s.DocFiles[0] != "README.md" {
		t.Errorf("expected 1 doc file, got: %v", s.DocFiles)
	}
	if len(s.OtherFiles) != 1 || s.OtherFiles[0] != "assets/logo.png" {
		t.Errorf("expected 1 other file, got: %v", s.OtherFiles)
	}
}

func TestParseStatOutput(t *testing.T) {
	s := &DiffSummary{}
	output := " pkg/agent/loop.go | 24 ++++\n scripts/metrics.sh | 83 ++++++++++\n 4 files changed, 478 insertions(+), 5 deletions(-)"
	parseStatOutput(output, s)

	if s.FilesChanged != 4 {
		t.Errorf("expected 4 files changed, got %d", s.FilesChanged)
	}
	if s.Insertions != 478 {
		t.Errorf("expected 478 insertions, got %d", s.Insertions)
	}
	if s.Deletions != 5 {
		t.Errorf("expected 5 deletions, got %d", s.Deletions)
	}
}

func TestGetLastDeployCommit_NoFile(t *testing.T) {
	// If the deploy marker doesn't exist, should return empty string
	// (we can't easily test the real file path, so just verify it doesn't panic)
	result := GetLastDeployCommit()
	// Result is either empty or a valid hash string
	if result != "" && len(result) < 4 {
		t.Errorf("unexpected short commit hash: %q", result)
	}
}

func TestGenerateSummary_RepoExists(t *testing.T) {
	// Test that GenerateSummary works on the actual Thor repo
	// This is an integration test that requires git to be available
	summary, err := GenerateSummary(ThorRepoPath)
	if err != nil {
		// Git might not be available in test environment — skip gracefully
		t.Skipf("git not available or repo not found: %v", err)
	}

	if summary == nil {
		t.Fatal("expected non-nil summary")
	}

	if summary.ToCommit == "" {
		t.Error("expected non-empty ToCommit")
	}

	if summary.FromCommit == "" {
		t.Error("expected non-empty FromCommit")
	}

	// Should have at least one commit in log
	if len(summary.CommitLog) == 0 {
		t.Error("expected at least one entry in CommitLog")
	}

	// Format should work without panic
	formatted := FormatSummary(summary)
	if formatted == "" {
		t.Error("expected non-empty formatted summary")
	}
}
