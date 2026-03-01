package legacy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"thor/pkg/config"
	"thor/pkg/migrate/internal"
)

var providerMapping = map[string]string{
	"anthropic":  "anthropic",
	"claude":     "anthropic",
	"openai":     "openai",
	"gpt":        "openai",
	"groq":       "groq",
	"ollama":     "ollama",
	"openrouter": "openrouter",
	"deepseek":   "deepseek",
	"together":   "together",
	"mistral":    "mistral",
	"fireworks":  "fireworks",
	"google":     "google",
	"gemini":     "google",
	"xai":        "xai",
	"grok":       "xai",
	"cerebras":   "cerebras",
	"sambanova":  "sambanova",
}

type LegacyHandler struct {
	opts             Options
	sourceConfigFile string
	sourceWorkspace  string
}

type (
	Options   = internal.Options
	Action    = internal.Action
	Result    = internal.Result
	Operation = internal.Operation
)

func NewLegacyHandler(opts Options) (Operation, error) {
	home, err := resolveSourceHome(opts.SourceHome)
	if err != nil {
		return nil, err
	}
	opts.SourceHome = home

	configFile, err := findSourceConfig(home)
	if err != nil {
		return nil, err
	}
	return &LegacyHandler{
		opts:             opts,
		sourceWorkspace:  filepath.Join(opts.SourceHome, "workspace"),
		sourceConfigFile: configFile,
	}, nil
}

func (o *LegacyHandler) GetSourceName() string {
	return "legacy"
}

func (o *LegacyHandler) GetSourceHome() (string, error) {
	return o.opts.SourceHome, nil
}

func (o *LegacyHandler) GetSourceWorkspace() (string, error) {
	return o.sourceWorkspace, nil
}

func (o *LegacyHandler) GetSourceConfigFile() (string, error) {
	return o.sourceConfigFile, nil
}

func (o *LegacyHandler) GetMigrateableFiles() []string {
	return migrateableFiles
}

func (o *LegacyHandler) GetMigrateableDirs() []string {
	return migrateableDirs
}

func (o *LegacyHandler) ExecuteConfigMigration(srcConfigPath, dstConfigPath string) error {
	legacyCfg, err := LoadLegacyConfig(srcConfigPath)
	if err != nil {
		return err
	}

	thorCfg, warnings, err := legacyCfg.ConvertToThor(o.opts.SourceHome)
	if err != nil {
		return err
	}

	for _, w := range warnings {
		fmt.Printf("  Warning: %s\n", w)
	}

	incoming := thorCfg.ToStandardConfig()
	if err := os.MkdirAll(filepath.Dir(dstConfigPath), 0o755); err != nil {
		return err
	}

	return config.SaveConfig(dstConfigPath, incoming)
}

func resolveSourceHome(override string) (string, error) {
	if override != "" {
		return internal.ExpandHome(override), nil
	}
	if envHome := os.Getenv("LEGACY_HOME"); envHome != "" {
		return internal.ExpandHome(envHome), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".legacy"), nil
}

func findSourceConfig(sourceHome string) (string, error) {
	candidates := []string{
		filepath.Join(sourceHome, "legacy.json"),
		filepath.Join(sourceHome, "config.json"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no config file found in %s (tried legacy.json, config.json)", sourceHome)
}

func rewriteWorkspacePath(path string) string {
	path = strings.Replace(path, ".legacy", ".thor", 1)
	return path
}

func mapProvider(provider string) string {
	if mapped, ok := providerMapping[strings.ToLower(provider)]; ok {
		return mapped
	}
	return strings.ToLower(provider)
}
