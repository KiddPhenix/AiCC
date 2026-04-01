package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	AI      AIConfig      `json:"ai"`
	Capture CaptureConfig `json:"capture"`
}

type AIConfig struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

type CaptureConfig struct {
	AutoAnalyze bool `json:"auto_analyze"`
}

func Load(path string) (*Config, error) {
	cfg := &Config{}

	if strings.TrimSpace(path) == "" {
		path = defaultPath()
	}

	raw, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(raw, cfg)
	}

	applyEnvOverrides(cfg)
	applyDefaults(cfg)
	return cfg, nil
}

func Save(path string, cfg *Config) error {
	if strings.TrimSpace(path) == "" {
		path = defaultPath()
	}
	applyDefaults(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o600)
}

func defaultPath() string {
	return filepath.Join("config", "app.json")
}

func applyEnvOverrides(cfg *Config) {
	if value := strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")); value != "" {
		cfg.AI.BaseURL = value
	}
	if value := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); value != "" {
		cfg.AI.APIKey = value
	}
	if value := strings.TrimSpace(os.Getenv("OPENAI_MODEL")); value != "" {
		cfg.AI.Model = value
	}
}

func applyDefaults(cfg *Config) {
	if strings.TrimSpace(cfg.AI.BaseURL) == "" {
		cfg.AI.BaseURL = "https://api.openai.com/v1"
	}
	if strings.TrimSpace(cfg.AI.Model) == "" {
		cfg.AI.Model = "gpt-4o-mini"
	}
}
