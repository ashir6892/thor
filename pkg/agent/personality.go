// personality.go — Adaptive Personality Profiler
// 🧠 Archimedes Cycle 3 — Detects user communication style and returns
// a personality hint injected into the system prompt.
package agent

import (
	"fmt"
	"strings"
	"unicode"

	"thor/pkg/providers"
)

// PersonalityProfile holds detected communication style for a session.
type PersonalityProfile struct {
	Verbosity     string // "terse", "normal", "verbose"
	Technicality  string // "casual", "mixed", "technical"
	Formality     string // "informal", "neutral", "formal"
	AvgUserMsgLen int
}

// DetectPersonality analyses recent conversation history and returns a profile.
// It requires at least 3 user messages to produce a meaningful profile.
// Returns nil if there is insufficient data.
func DetectPersonality(history []providers.Message) *PersonalityProfile {
	userMessages := make([]string, 0, len(history))
	for _, m := range history {
		if m.Role == "user" {
			if strings.HasPrefix(m.Content, "[System:") {
				continue
			}
			userMessages = append(userMessages, m.Content)
		}
	}

	if len(userMessages) < 3 {
		return nil
	}

	totalLen := 0
	for _, msg := range userMessages {
		totalLen += len([]rune(msg))
	}
	avgLen := totalLen / len(userMessages)

	verbosity := "normal"
	switch {
	case avgLen < 30:
		verbosity = "terse"
	case avgLen > 120:
		verbosity = "verbose"
	}

	techScore := 0
	techKeywords := []string{
		"function", "error", "code", "api", "json", "http", "server",
		"goroutine", "package", "import", "struct", "interface", "nil",
		"bash", "curl", "grep", "awk", "python", "golang", "linux",
		"docker", "git", "deploy", "binary", "compile", "debug",
		"config", "yaml", "toml", "env", "variable", "regex",
	}
	combined := strings.ToLower(strings.Join(userMessages, " "))
	for _, kw := range techKeywords {
		if strings.Contains(combined, kw) {
			techScore++
		}
	}

	technicality := "casual"
	switch {
	case techScore >= 8:
		technicality = "technical"
	case techScore >= 3:
		technicality = "mixed"
	}

	informalScore := 0
	formalScore := 0
	informalIndicators := []string{"hey", "hi", "lol", "haha", "ok", "yeah", "nope", "btw", "tbh", "omg"}
	formalIndicators := []string{"please", "could you", "would you", "kindly", "thank you", "regards", "sincerely"}

	for _, ind := range informalIndicators {
		if strings.Contains(combined, ind) {
			informalScore++
		}
	}
	for _, ind := range formalIndicators {
		if strings.Contains(combined, ind) {
			formalScore++
		}
	}

	for _, r := range combined {
		if r > unicode.MaxASCII {
			informalScore++
			break
		}
	}

	formality := "neutral"
	if informalScore > formalScore+1 {
		formality = "informal"
	} else if formalScore > informalScore {
		formality = "formal"
	}

	return &PersonalityProfile{
		Verbosity:     verbosity,
		Technicality:  technicality,
		Formality:     formality,
		AvgUserMsgLen: avgLen,
	}
}

// FormatHint returns a concise system-prompt injection string based on the profile.
// Returns empty string if profile is nil or all dimensions are balanced.
func (p *PersonalityProfile) FormatHint() string {
	if p == nil {
		return ""
	}

	if p.Verbosity == "normal" && p.Technicality == "mixed" && p.Formality == "neutral" {
		return ""
	}

	var hints []string

	switch p.Verbosity {
	case "terse":
		hints = append(hints, "keep responses SHORT and direct — this user prefers brevity")
	case "verbose":
		hints = append(hints, "this user appreciates thorough, detailed explanations")
	}

	switch p.Technicality {
	case "technical":
		hints = append(hints, "use technical language freely — code snippets, exact commands, precise terminology")
	case "casual":
		hints = append(hints, "use plain, everyday language — avoid jargon and technical details unless asked")
	}

	switch p.Formality {
	case "informal":
		hints = append(hints, "match the user's casual, conversational tone")
	case "formal":
		hints = append(hints, "maintain a professional, polished tone")
	}

	if len(hints) == 0 {
		return ""
	}

	return fmt.Sprintf("\n\n[Adaptive Style: %s]", strings.Join(hints, "; "))
}
