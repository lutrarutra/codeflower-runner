package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Manager struct {
	configDir string
	cacheDir  string
}

type Config struct {
	APIToken string `json:"api_token"`
}

func (m *Manager) SaveAPIToken(token string) error {
	return m.SaveConfig("config.json", Config{APIToken: token})
}

func (m *Manager) GetAPIToken() (string, error) {
	var cfg Config
	err := m.LoadConfig("config.json", &cfg)
	if err != nil {
		return "", err
	}
	return cfg.APIToken, nil
}

func NewManager() (*Manager, error) {
	conf, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config dir: %w", err)
	}

	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get cache dir: %w", err)
	}

	m := &Manager{
		configDir: filepath.Join(conf, "codeflower"),
		cacheDir:  filepath.Join(cache, "codeflower"),
	}

	// Ensure directories exist
	if err := os.MkdirAll(m.configDir, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(m.cacheDir, 0755); err != nil {
		return nil, err
	}

	return m, nil
}

// --- Configuration (Persistent) ---

func (m *Manager) SaveConfig(filename string, data interface{}) error {
	path := filepath.Join(m.configDir, filename)
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0600) // Restricted permissions for config
}

func (m *Manager) LoadConfig(filename string, target interface{}) error {
	path := filepath.Join(m.configDir, filename)
	bytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, target)
}

// --- Cache (Temporary) ---

func (m *Manager) SetCache(key string, data interface{}) error {
	path := filepath.Join(m.cacheDir, key+".json")
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0644)
}

func (m *Manager) GetCache(key string, target interface{}, maxAge time.Duration) error {
	path := filepath.Join(m.cacheDir, key+".json")
	info, err := os.Stat(path)
	if err != nil {
		return err // Not found
	}

	if time.Since(info.ModTime()) > maxAge {
		return fmt.Errorf("cache expired")
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, target)
}
