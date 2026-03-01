package configstore

import (
	"errors"
	"os"
	"path/filepath"

	thorconfig "thor/pkg/config"
)

const (
	configDirName  = ".thor"
	configFileName = "config.json"
)

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDirName), nil
}

func Load() (*thorconfig.Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	return thorconfig.LoadConfig(path)
}

func Save(cfg *thorconfig.Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	return thorconfig.SaveConfig(path, cfg)
}
