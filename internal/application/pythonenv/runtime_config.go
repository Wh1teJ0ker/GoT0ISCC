package pythonenv

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

type fallbackConfig struct {
	Enabled   bool                            `json:"enabled"`
	Platforms map[string]fallbackPlatformSpec `json:"platforms"`
}

type fallbackPlatformSpec struct {
	URL         string `json:"url"`
	ArchiveType string `json:"archive_type"`
	SHA256      string `json:"sha256"`
	BinaryPath  string `json:"binary_path"`
}

const metaPythonFallbackConfig = "pythonenv.fallback_config"

func (s *Service) fallbackConfigPath() string {
	return "sqlite:meta/pythonenv.fallback_config"
}

func (s *Service) fallbackRuntimeRoot() string {
	return filepath.Join(s.layout.AppDataRoot, "python-runtime")
}

func (s *Service) loadFallbackConfig() (fallbackConfig, error) {
	config := fallbackConfig{
		Enabled:   false,
		Platforms: map[string]fallbackPlatformSpec{},
	}
	raw, err := s.store.MetaValue(context.Background(), metaPythonFallbackConfig)
	if err != nil {
		return fallbackConfig{}, fmt.Errorf("读取 Python fallback 配置失败: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return config, nil
	}
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return fallbackConfig{}, fmt.Errorf("解析 Python fallback 配置失败: %w", err)
	}
	if config.Platforms == nil {
		config.Platforms = map[string]fallbackPlatformSpec{}
	}
	return config, nil
}

func (c fallbackConfig) platformKey() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

func (c fallbackConfig) platformSpec() (fallbackPlatformSpec, bool) {
	spec, ok := c.Platforms[c.platformKey()]
	return spec, ok
}

func (c fallbackConfig) isConfiguredForCurrentPlatform() bool {
	spec, ok := c.platformSpec()
	if !ok {
		return false
	}
	return strings.TrimSpace(spec.URL) != "" && strings.TrimSpace(spec.ArchiveType) != "" && strings.TrimSpace(spec.BinaryPath) != ""
}
