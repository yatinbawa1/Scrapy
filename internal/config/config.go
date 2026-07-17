package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Config struct {
	mu             sync.RWMutex `json:"-"`
	FirstRun       bool         `json:"firstRun"`
	DataDir        string       `json:"dataDir"`
	CacheDir       string       `json:"cacheDir"`
	ThumbnailDir   string       `json:"thumbnailDir"`
	DownloadDir    string       `json:"downloadDir"`
	MaxCacheSizeMB int          `json:"maxCacheSizeMB"`
	ConcurrentDl   int          `json:"concurrentDl"`
	Theme          string       `json:"theme"`
	DBPath         string       `json:"dbPath"`
	EnabledSources []string     `json:"enabledSources"`
	SearchTerms    []string     `json:"searchTerms"`
}

var defaultSearchTerms = []string{
	"mountains", "ocean", "forest", "city", "sunset",
	"space", "nature", "minimal", "abstract", "dark",
	"cars", "animals", "flowers", "aerial", "desert",
	"night", "beach", "winter", "rain", "clouds",
}

var defaultConfig = &Config{
	FirstRun:       true,
	MaxCacheSizeMB: 5000,
	ConcurrentDl:   10,
	Theme:          "system",
	EnabledSources: []string{"wallhaven"},
	SearchTerms:    defaultSearchTerms,
}

func configPath(appDir string) string {
	return filepath.Join(appDir, "config.json")
}

func New(appDir string) (*Config, error) {
	cfg := *defaultConfig
	cfg.DataDir = appDir
	cfg.CacheDir = filepath.Join(appDir, "cache")
	cfg.ThumbnailDir = filepath.Join(appDir, "cache", "thumbnails")
	cfg.DownloadDir = filepath.Join(appDir, "downloads")
	cfg.DBPath = filepath.Join(appDir, "wallpapers.db")

	for _, d := range []string{cfg.CacheDir, cfg.ThumbnailDir, cfg.DownloadDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return nil, err
		}
	}

	existing, err := loadFromFile(appDir)
	if err == nil {
		existing.DataDir = cfg.DataDir
		existing.CacheDir = cfg.CacheDir
		existing.ThumbnailDir = cfg.ThumbnailDir
		existing.DBPath = cfg.DBPath
		return existing, nil
	}

	cfg.persist(appDir)
	return &cfg, nil
}

func loadFromFile(appDir string) (*Config, error) {
	data, err := os.ReadFile(configPath(appDir))
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) persist(appDir string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(configPath(appDir), data, 0644)
}

func (c *Config) Save(appDir string) error {
	c.persist(appDir)
	return nil
}

func (c *Config) Reset(appDir string) {
	*c = *defaultConfig
	c.DataDir = appDir
	c.CacheDir = filepath.Join(appDir, "cache")
	c.ThumbnailDir = filepath.Join(appDir, "cache", "thumbnails")
	c.DownloadDir = filepath.Join(appDir, "downloads")
	c.DBPath = filepath.Join(appDir, "wallpapers.db")
	c.persist(appDir)
}

func (c *Config) SetFirstRun(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.FirstRun = v
}

func (c *Config) SetDownloadDir(v string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.DownloadDir = v
	os.MkdirAll(v, 0755)
}

func (c *Config) SetEnabledSources(v []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.EnabledSources = v
}

func (c *Config) SetMaxCacheSizeMB(v int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.MaxCacheSizeMB = v
}

func (c *Config) SetConcurrentDl(v int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ConcurrentDl = v
}

func (c *Config) SetTheme(v string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Theme = v
}

func (c *Config) GetSearchTerms() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]string, len(c.SearchTerms))
	copy(result, c.SearchTerms)
	return result
}

func (c *Config) AddSearchTerm(term string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range c.SearchTerms {
		if t == term {
			return false
		}
	}
	c.SearchTerms = append(c.SearchTerms, term)
	return true
}

func (c *Config) RemoveSearchTerm(term string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, t := range c.SearchTerms {
		if t == term {
			c.SearchTerms = append(c.SearchTerms[:i], c.SearchTerms[i+1:]...)
			return true
		}
	}
	return false
}

func GetDefaultAppDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "wallpaper-chooser")
}
