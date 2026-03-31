package config

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

type Config struct {
	TMVersion string `json:"tm_version,omitempty"`
	Diet      string `json:"diet,omitempty"`
	MaxTime   int    `json:"max_time,omitempty"`
}

func Dir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "tmx")
}

func path(name string) string {
	return filepath.Join(Dir(), name)
}

func Load() (*Config, error) {
	data, err := os.ReadFile(path("config.json"))
	if os.IsNotExist(err) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) Save() error {
	if err := os.MkdirAll(Dir(), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path("config.json"), data, 0600)
}

func (c *Config) Reset() error {
	*c = Config{}
	return c.Save()
}

// Cookies

func LoadCookies() ([]*http.Cookie, error) {
	data, err := os.ReadFile(path("cookies.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cookies []*http.Cookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return nil, err
	}
	return cookies, nil
}

func SaveCookies(cookies []*http.Cookie) error {
	if err := os.MkdirAll(Dir(), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cookies, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path("cookies.json"), data, 0600)
}

func ClearCookies() error {
	err := os.Remove(path("cookies.json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Generic JSON cache helpers

func LoadCache(name string, v any) error {
	data, err := os.ReadFile(path(name))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func SaveCache(name string, v any) error {
	if err := os.MkdirAll(Dir(), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path(name), data, 0600)
}

func ClearCache(name string) error {
	err := os.Remove(path(name))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
