package config

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Orgs            []string      `yaml:"orgs"`
	Repos           []string      `yaml:"repos"`
	Author          string        `yaml:"author"`
	MergeMethod     string        `yaml:"merge_method"`
	RefreshInterval time.Duration `yaml:"refresh_interval"`
}

func defaults() Config {
	return Config{
		Author:          "renovate[bot]",
		MergeMethod:     "squash",
		RefreshInterval: 5 * time.Minute,
	}
}

func Load() (*Config, error) {
	cfg := defaults()

	path, err := configPath()
	if err != nil {
		return &cfg, nil
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &cfg, nil
	}
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if cfg.Author == "" {
		cfg.Author = "renovate[bot]"
	}
	if cfg.MergeMethod == "" {
		cfg.MergeMethod = "squash"
	}
	if cfg.RefreshInterval == 0 {
		cfg.RefreshInterval = 5 * time.Minute
	}

	return &cfg, nil
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "gh-renovate-tracker", "config.yaml"), nil
}
