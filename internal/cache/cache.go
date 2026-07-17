package cache

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"wallpaper-chooser/internal/database"
)

type Cache struct {
	db         *database.DB
	cacheDir   string
	thumbDir   string
	maxSizeMB  int
	mu         sync.Mutex
}

func New(db *database.DB, cacheDir, thumbDir string, maxSizeMB int) *Cache {
	return &Cache{
		db:        db,
		cacheDir:  cacheDir,
		thumbDir:  thumbDir,
		maxSizeMB: maxSizeMB,
	}
}

func (c *Cache) EnforceLimit() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.maxSizeMB <= 0 {
		return nil
	}

	totalSize, files, err := c.dirSize(c.cacheDir)
	if err != nil {
		return err
	}

	maxBytes := int64(c.maxSizeMB) * 1024 * 1024
	if totalSize <= maxBytes {
		return nil
	}

	sort.Slice(files, func(i, j int) bool {
		fi, _ := os.Stat(files[i])
		fj, _ := os.Stat(files[j])
		if fi == nil || fj == nil {
			return false
		}
		return fi.ModTime().Before(fj.ModTime())
	})

	for _, f := range files {
		if totalSize <= maxBytes {
			break
		}
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		if err := os.Remove(f); err != nil {
			log.Printf("[cache] remove %s: %v", f, err)
			continue
		}
		totalSize -= info.Size()
		log.Printf("[cache] evicted %s (%d bytes)", f, info.Size())
	}

	return nil
}

func (c *Cache) dirSize(path string) (int64, []string, error) {
	var totalSize int64
	var files []string

	err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			totalSize += info.Size()
			files = append(files, p)
		}
		return nil
	})

	return totalSize, files, err
}

func (c *Cache) CurrentSizeMB() float64 {
	size, _, _ := c.dirSize(c.cacheDir)
	return float64(size) / (1024 * 1024)
}

func (c *Cache) Cleanup() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	removed := 0
	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		return 0
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) > 30*24*time.Hour {
			os.Remove(filepath.Join(c.cacheDir, entry.Name()))
			removed++
		}
	}

	return removed
}

func (c *Cache) DeleteAll() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		os.RemoveAll(filepath.Join(c.cacheDir, entry.Name()))
	}

	thumbEntries, err := os.ReadDir(c.thumbDir)
	if err != nil {
		return err
	}

	for _, entry := range thumbEntries {
		os.RemoveAll(filepath.Join(c.thumbDir, entry.Name()))
	}

	return nil
}
