// Package shared
package shared

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
)

const MetadataRepo = "tissla/opfor-metadata-barrys"

var defaultSource = ScraperConfig{
	Name: "nyaa", BaseURL: "https://nyaa.si",
	SearchPathTemplate: "/?f=0&c=1_2&q=%s&p=%d", SearchQuery: "one+pace",
	RowSelector: "table tbody tr",
	Fields:      ScraperFields{Title: "td:nth-child(2) a:last-child", Seeders: "td:nth-child(6)", TorrentLink: "td:nth-child(3) a[href^='/download/']", TorrentID: `/download/(\d+)`, UploadDate: "td:nth-child(5)", FileSize: "td:nth-child(4)"},
	Validation:  ValidationConfig{RequiredInTitle: "one pace"},
}

var defaultCfg = Config{
	TargetDir:  "",
	GitHubRepo: MetadataRepo,
	Source:     defaultSource,
	Thumbnails: false,
}

var (
	globalConfig    *Config
	configLoadOnce  sync.Once
	configLoadError error
)

// LoadConfig loads config-file file, creates it if it does not exist
func LoadConfig() (*Config, error) {

	configLoadOnce.Do(func() {
		path := EnsureConfigExists()

		data, err := os.ReadFile(path)
		if err != nil {
			configLoadError = err
		}

		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			configLoadError = err
		}
		cfg.GitHubRepo = MetadataRepo
		cfg.Source = withSourceDefaults(cfg.Source)
		globalConfig = &cfg
	})

	if configLoadError != nil {
		return nil, configLoadError
	}

	return globalConfig, nil
}

// withSourceDefaults fills in a missing or partial scraper config. Configs saved
// from the old metadata repo have no upload_date or file_size selectors, and the
// current metadata repo ships no scraper config that would refresh them.
func withSourceDefaults(src ScraperConfig) ScraperConfig {
	if src.BaseURL == "" {
		return defaultSource
	}
	if src.Name == defaultSource.Name {
		if src.Fields.UploadDate == "" {
			src.Fields.UploadDate = defaultSource.Fields.UploadDate
		}
		if src.Fields.FileSize == "" {
			src.Fields.FileSize = defaultSource.Fields.FileSize
		}
	}
	return src
}

// SaveConfig writes static config file from config object, as json
func SaveConfig(cfg Config) error {
	path := getConfigPath()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}

	globalConfig = &cfg

	return nil
}

// EnsureConfigExists creates default config if no config-file is found
func EnsureConfigExists() string {
	path := getConfigPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {

		SaveConfig(defaultCfg)
	}

	return path
}

// getConfigPath returns the config filepath from the OS's default config directory
func getConfigPath() string {
	return filepath.Join(ConfigDir(), "config.json")
}

// ConfigDir returns the app's config directory
func ConfigDir() string {
	dirname, err := os.UserConfigDir()
	if err != nil {
		log.Fatalf("could not determine config directory: %v", err)
	}
	path := filepath.Join(dirname, "opforjellyfin")
	if err := os.MkdirAll(path, 0755); err != nil {
		log.Fatalf("could not create config dir: %v", err)
	}
	return path
}
