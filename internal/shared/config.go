// Package shared
package shared

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// DefaultMetadataRepo is used when the config names no metadata repo.
const DefaultMetadataRepo = "tissla/opfor-metadata-barrys"

// legacyMetadataRepo uses an index format this version cannot read.
const legacyMetadataRepo = "tissla/one-pace-jellyfin"

var githubRepoName = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})/[A-Za-z0-9._-]{1,100}$`)

// ValidateMetadataRepo checks a GitHub "owner/name" repo, which ends up in a git clone URL.
func ValidateMetadataRepo(repo string) error {
	name := repo[strings.Index(repo, "/")+1:]
	if !githubRepoName.MatchString(repo) || name == "." || name == ".." {
		return fmt.Errorf("invalid metadata repo %q: expected a GitHub repo as owner/name", repo)
	}
	return nil
}

// MetadataRepo returns the configured metadata repo, falling back to the default
// for an empty value or the legacy repo saved by older versions.
func (c *Config) MetadataRepo() string {
	repo := strings.TrimSpace(c.GitHubRepo)
	if repo == "" || strings.EqualFold(repo, legacyMetadataRepo) {
		return DefaultMetadataRepo
	}
	return repo
}

var defaultSource = ScraperConfig{
	Name: "nyaa", BaseURL: "https://nyaa.si",
	SearchPathTemplate: "/?f=0&c=1_2&q=%s&p=%d", SearchQuery: "one+pace",
	RowSelector: "table tbody tr",
	Fields:      ScraperFields{Title: "td:nth-child(2) a:last-child", Seeders: "td:nth-child(6)", TorrentLink: "td:nth-child(3) a[href^='/download/']", TorrentID: `/download/(\d+)`, UploadDate: "td:nth-child(5)", FileSize: "td:nth-child(4)"},
	Validation:  ValidationConfig{RequiredInTitle: "one pace"},
}

var defaultCfg = Config{
	TargetDir:  "",
	GitHubRepo: DefaultMetadataRepo,
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
		cfg.GitHubRepo = cfg.MetadataRepo()
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
