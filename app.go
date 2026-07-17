package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"wallpaper-chooser/internal/cache"
	"wallpaper-chooser/internal/config"
	"wallpaper-chooser/internal/database"
	"wallpaper-chooser/internal/downloader"
	"wallpaper-chooser/internal/scraper"
	"wallpaper-chooser/internal/search"
	"wallpaper-chooser/internal/thumbnail"
	wallpaperpkg "wallpaper-chooser/internal/wallpaper"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var defaultSearchTerms = []string{
	"mountains", "ocean", "forest", "city", "sunset",
	"space", "nature", "minimal", "abstract", "dark",
	"cars", "animals", "flowers", "aerial", "desert",
	"night", "beach", "winter", "rain", "clouds",
}

type App struct {
	ctx        context.Context
	cfg        *config.Config
	db         *database.DB
	scraper    *scraper.Engine
	dl         *downloader.Downloader
	cache      *cache.Cache
	thumbs     *thumbnail.Generator
	search     *search.Engine
	mu         sync.Mutex
	appDir     string
	scrapeMu   sync.Mutex
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	a.appDir = config.GetDefaultAppDir()
	if err := os.MkdirAll(a.appDir, 0755); err != nil {
		log.Fatalf("create app dir: %v", err)
	}

	cfg, err := config.New(a.appDir)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	a.cfg = cfg

	db, err := database.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	a.db = db

	se := scraper.NewEngine()
	se.Register(scraper.NewWallhaven(se))
	se.Register(scraper.NewUnsplash(se))
	se.Register(scraper.NewPexels(se))
	a.scraper = se

	a.dl = downloader.New(db, cfg.CacheDir, cfg.ThumbnailDir, cfg.ConcurrentDl)
	a.dl.Start()

	a.cache = cache.New(db, cfg.CacheDir, cfg.ThumbnailDir, cfg.MaxCacheSizeMB)
	a.thumbs = thumbnail.New(cfg.ThumbnailDir)
	a.search = search.New(db)

	go a.thumbnailDownloader()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[app] progress handler recovered: %v", r)
			}
		}()
		for p := range a.dl.ProgressChan() {
			switch p.Status {
			case "downloaded":
				w, err := a.db.GetWallpaper(p.WallpaperID)
				if err == nil && w.LocalPath != "" {
					ext := filepath.Ext(w.LocalPath)
					thumbPath := filepath.Join(a.cfg.ThumbnailDir, fmt.Sprintf("%d%s", w.ID, ext))
					if err := a.thumbs.Generate(w.LocalPath, thumbPath); err == nil {
						a.db.UpdateThumbnailPath(w.ID, thumbPath)
						log.Printf("[app] generated thumbnail for %d", w.ID)
					}
				}
				runtime.EventsEmit(a.ctx, "wallpaper:downloaded", p)
			case "failed", "duplicate":
				runtime.EventsEmit(a.ctx, "download:failed", p)
			}
		}
	}()

	log.Printf("[app] started, data dir: %s", a.appDir)
}

func (a *App) shutdown(ctx context.Context) {
	a.dl.Stop()
	if a.db != nil {
		a.db.Close()
	}
}

func (a *App) thumbnailDownloader() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[app] thumbnailDownloader recovered: %v", r)
		}
	}()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		pending, err := a.db.GetScrapedWithoutThumbnail(50)
		if err != nil {
			continue
		}
		for _, w := range pending {
			a.dl.EnqueueThumbnail(w)
		}
		if len(pending) > 0 {
			runtime.EventsEmit(a.ctx, "thumbnail:batch", map[string]int{"count": len(pending)})
		}
	}
}

func (a *App) IsFirstRun() bool {
	return a.cfg.FirstRun
}

func (a *App) SelectFolder() string {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Wallpaper Storage Folder",
	})
	if err != nil || dir == "" {
		return ""
	}
	return dir
}

func (a *App) CompleteOnboarding(downloadDir string, sources []string, concurrentDl int) error {
	if downloadDir != "" {
		a.cfg.SetDownloadDir(downloadDir)
	}
	if len(sources) > 0 {
		a.cfg.SetEnabledSources(sources)
	}
	if concurrentDl > 0 {
		a.cfg.SetConcurrentDl(concurrentDl)
	}

	a.cfg.SetFirstRun(false)
	a.cfg.Save(a.appDir)

	log.Printf("[app] onboarding complete: dir=%s sources=%v concurrency=%d", downloadDir, sources, concurrentDl)

	runtime.EventsEmit(a.ctx, "scrape:total", map[string]interface{}{"total": len(sources), "sources": sources})
	for _, src := range sources {
		go a.scrapeSource(src)
	}

	return nil
}

func (a *App) scrapeSource(source string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[app] scrapeSource %s recovered: %v", source, r)
		}
	}()
	log.Printf("[app] scraping %s...", source)
	runtime.EventsEmit(a.ctx, "scrape:started", map[string]string{"source": source})

	totalAdded := 0
	for _, term := range a.cfg.GetSearchTerms() {
		for page := 1; page <= 3; page++ {
			wallpapers, _, err := a.scraper.ScrapeSearch(source, term, page)
			if err != nil {
				log.Printf("[app] scrape %s/%s page %d: %v", source, term, page, err)
				break
			}

			added := 0
			for _, w := range wallpapers {
				if a.db.ExistsByURL(w.URL) {
					continue
				}
				if _, err := a.db.InsertWallpaper(&w); err == nil {
					added++
				}
			}
			totalAdded += added
			runtime.EventsEmit(a.ctx, "scrape:progress", map[string]interface{}{
				"source":    source,
				"term":      term,
				"page":      page,
				"added":     added,
				"total":     totalAdded,
			})

			time.Sleep(200 * time.Millisecond)
		}
	}

	log.Printf("[app] scrape %s complete: %d wallpapers added", source, totalAdded)
	runtime.EventsEmit(a.ctx, "scrape:complete", map[string]interface{}{"source": source, "added": totalAdded})
}

func (a *App) ScrapeAll(page int) map[string]int {
	results := map[string]int{}
	sources := a.cfg.EnabledSources
	if len(sources) == 0 {
		sources = a.scraper.Providers()
	}

	runtime.EventsEmit(a.ctx, "scrape:total", map[string]interface{}{"total": len(sources), "sources": sources})

	for _, name := range sources {
		go a.scrapeSource(name)
	}

	return results
}

func (a *App) DownloadAndSetWallpaper(id int64) error {
	w, err := a.db.GetWallpaper(id)
	if err != nil {
		return err
	}

	if w.Status == "downloaded" && w.LocalPath != "" {
		return wallpaperpkg.SetWallpaper(w.LocalPath)
	}

	a.db.UpdateStatus(id, "downloading")
	a.dl.EnqueueFull(*w)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[app] DownloadAndSetWallpaper %d recovered: %v", id, r)
			}
		}()
		for i := 0; i < 120; i++ {
			time.Sleep(1 * time.Second)
			updated, err := a.db.GetWallpaper(id)
			if err != nil {
				continue
			}
			if updated.Status == "downloaded" && updated.LocalPath != "" {
				runtime.EventsEmit(a.ctx, "wallpaper:downloaded", downloader.Progress{WallpaperID: id, Status: "downloaded"})
				if err := wallpaperpkg.SetWallpaper(updated.LocalPath); err != nil {
					log.Printf("[app] set wallpaper %d: %v", id, err)
				}
				return
			}
			if updated.Status == "failed" || updated.Status == "duplicate" {
				runtime.EventsEmit(a.ctx, "download:failed", downloader.Progress{WallpaperID: id, Status: updated.Status})
				return
			}
		}
	}()

	return nil
}

func (a *App) DownloadWallpaper(id int64) error {
	w, err := a.db.GetWallpaper(id)
	if err != nil {
		return err
	}
	if w.Status == "downloaded" {
		return nil
	}
	a.db.UpdateStatus(id, "downloading")
	a.dl.EnqueueFull(*w)
	return nil
}

func (a *App) CancelDownload(id int64) {
	a.dl.CancelDownload(id)
}

func (a *App) Search(query, source string, minW, minH, page, pageSize int) database.SearchResult {
	result, err := a.search.Search(query, source, minW, minH, page, pageSize)
	if err != nil {
		log.Printf("[app] search: %v", err)
		return database.SearchResult{}
	}
	return result
}

func (a *App) Browse(page, pageSize int, favorites bool) []database.Wallpaper {
	wallpapers, _, _ := a.search.BrowseSortedFiltered(page, pageSize, favorites, "latest", "", "", "")
	return wallpapers
}

func (a *App) BrowseSorted(page, pageSize int, favorites bool, sortBy string) []database.Wallpaper {
	wallpapers, _, _ := a.search.BrowseSortedFiltered(page, pageSize, favorites, sortBy, "", "", "")
	return wallpapers
}

func (a *App) BrowseSortedFiltered(page, pageSize int, favorites bool, sortBy string, searchTerm string, query string, source string) database.SearchResult {
	wallpapers, total, err := a.search.BrowseSortedFiltered(page, pageSize, favorites, sortBy, searchTerm, query, source)
	if err != nil {
		log.Printf("[app] browse: %v", err)
		return database.SearchResult{}
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	return database.SearchResult{
		Wallpapers: wallpapers,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
	}
}

func (a *App) BrowseTotalCount(pageSize int, favorites bool, sortBy string, searchTerm string, query string, source string) int {
	_, total, err := a.search.BrowseSortedFiltered(1, pageSize, favorites, sortBy, searchTerm, query, source)
	if err != nil {
		return 0
	}
	return total
}

func (a *App) GetDownloadedWallpapers(page, pageSize int, sortBy string) []database.Wallpaper {
	if pageSize <= 0 {
		pageSize = 50
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize
	wallpapers, err := a.db.GetDownloadedSorted(pageSize, offset, sortBy)
	if err != nil {
		log.Printf("[app] get downloaded: %v", err)
		return nil
	}
	return wallpapers
}

func (a *App) GetWallpaper(id int64) *database.Wallpaper {
	w, err := a.db.GetWallpaper(id)
	if err != nil {
		return nil
	}
	return w
}

func (a *App) ToggleFavorite(id int64) error {
	w, err := a.db.GetWallpaper(id)
	if err != nil {
		return err
	}
	return a.db.SetFavorite(id, !w.IsFavorite)
}

func (a *App) SetWallpaper(id int64) error {
	return a.DownloadAndSetWallpaper(id)
}

func (a *App) DeleteWallpaper(id int64) error {
	w, err := a.db.GetWallpaper(id)
	if err != nil {
		return err
	}
	if w.LocalPath != "" {
		os.Remove(w.LocalPath)
	}
	if w.ThumbnailPath != "" {
		os.Remove(w.ThumbnailPath)
	}
	return a.db.DeleteWallpaper(id)
}

func (a *App) GetSources() []database.Source {
	sources, _ := a.db.GetSources()
	return sources
}

func (a *App) GetSourceStats() []map[string]interface{} {
	stats, _ := a.db.GetSourceStats()
	return stats
}

func (a *App) GetCategoryStats() []map[string]interface{} {
	stats, _ := a.db.GetCategoryStats()
	return stats
}

func (a *App) GetSearchTerms() []string {
	return a.cfg.GetSearchTerms()
}

func (a *App) AddSearchTerm(term string) bool {
	result := a.cfg.AddSearchTerm(term)
	if result {
		a.cfg.Save(a.appDir)
	}
	return result
}

func (a *App) RemoveSearchTerm(term string) bool {
	result := a.cfg.RemoveSearchTerm(term)
	if result {
		a.cfg.Save(a.appDir)
	}
	return result
}

func (a *App) GetStats() map[string]interface{} {
	stats, _ := a.db.GetStats()
	stats["cacheSizeMB"] = a.cache.CurrentSizeMB()
	return stats
}

func (a *App) GetProviders() []string {
	return a.scraper.Providers()
}

func (a *App) GetConfig() *config.Config {
	return a.cfg
}

func (a *App) ToggleSource(name string) {
	sources := a.cfg.EnabledSources
	found := false
	for i, s := range sources {
		if s == name {
			sources = append(sources[:i], sources[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		sources = append(sources, name)
	}
	a.cfg.SetEnabledSources(sources)
	a.cfg.Save(a.appDir)
}

func (a *App) IsSourceEnabled(name string) bool {
	for _, s := range a.cfg.EnabledSources {
		if s == name {
			return true
		}
	}
	return false
}

func (a *App) SetMaxCacheSizeMB(v int) {
	a.cfg.SetMaxCacheSizeMB(v)
	a.cache.EnforceLimit()
	a.cfg.Save(a.appDir)
}

func (a *App) SetConcurrentDownloads(v int) {
	a.cfg.SetConcurrentDl(v)
	a.cfg.Save(a.appDir)
}

func (a *App) GetDownloadQueue() map[string]int {
	return map[string]int{
		"active":  a.dl.ActiveCount(),
		"pending": a.dl.QueueCount(),
	}
}

type StorageItem struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Path      string `json:"path"`
	SizeBytes int64  `json:"sizeBytes"`
	Count     int    `json:"count"`
}

func pathSize(path string) (int64, int) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0
	}
	if !info.IsDir() {
		return info.Size(), 1
	}
	var total int64
	count := 0
	filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		total += fi.Size()
		count++
		return nil
	})
	return total, count
}

func (a *App) GetStorageInfo() []StorageItem {
	items := []StorageItem{
		{Key: "database", Label: "Database", Path: a.cfg.DBPath},
		{Key: "config", Label: "Config", Path: filepath.Join(a.appDir, "config.json")},
		{Key: "cache", Label: "Wallpaper Cache", Path: a.cfg.CacheDir},
		{Key: "thumbnails", Label: "Thumbnails", Path: a.cfg.ThumbnailDir},
		{Key: "downloads", Label: "Downloads", Path: a.cfg.DownloadDir},
	}
	for i := range items {
		size, count := pathSize(items[i].Path)
		if items[i].Key == "database" {
			for _, extra := range []string{items[i].Path + "-wal", items[i].Path + "-shm"} {
				s, c := pathSize(extra)
				size += s
				count += c
			}
		}
		items[i].SizeBytes = size
		items[i].Count = count
	}
	return items
}

func (a *App) removeDirContents(dir string, keepSubdirs bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if keepSubdirs && e.IsDir() {
			continue
		}
		os.RemoveAll(filepath.Join(dir, e.Name()))
	}
}

func (a *App) ClearStorage(key string) error {
	switch key {
	case "cache":
		a.removeDirContents(a.cfg.CacheDir, true)
	case "thumbnails":
		a.removeDirContents(a.cfg.ThumbnailDir, false)
	case "downloads":
		a.removeDirContents(a.cfg.DownloadDir, false)
	case "config":
		a.cfg.Reset(a.appDir)
	case "database":
		if err := a.db.DeleteAllWallpapers(); err != nil {
			return err
		}
		a.cache.EnforceLimit()
	default:
		return fmt.Errorf("unknown storage key: %s", key)
	}
	runtime.EventsEmit(a.ctx, "storage:updated", nil)
	return nil
}

func (a *App) ClearAllStorage() error {
	a.removeDirContents(a.cfg.CacheDir, true)
	a.removeDirContents(a.cfg.ThumbnailDir, false)
	a.removeDirContents(a.cfg.DownloadDir, false)
	a.cfg.Reset(a.appDir)
	if err := a.db.DeleteAllWallpapers(); err != nil {
		return err
	}
	a.cache.EnforceLimit()
	runtime.EventsEmit(a.ctx, "storage:updated", nil)
	return nil
}

func (a *App) CleanupCache() int {
	return a.cache.Cleanup()
}

func (a *App) ResetDatabase() error {
	a.dl.Stop()
	a.db.Close()

	dbPath := a.cfg.DBPath
	os.Remove(dbPath)
	os.Remove(dbPath + "-wal")
	os.Remove(dbPath + "-shm")

	os.RemoveAll(a.cfg.CacheDir)
	os.RemoveAll(a.cfg.ThumbnailDir)
	os.RemoveAll(a.cfg.DownloadDir)
	os.MkdirAll(a.cfg.CacheDir, 0755)
	os.MkdirAll(a.cfg.ThumbnailDir, 0755)
	os.MkdirAll(a.cfg.DownloadDir, 0755)

	db, err := database.New(dbPath)
	if err != nil {
		return err
	}
	a.db = db

	a.dl = downloader.New(db, a.cfg.CacheDir, a.cfg.ThumbnailDir, a.cfg.ConcurrentDl)
	a.dl.Start()
	a.thumbs = thumbnail.New(a.cfg.ThumbnailDir)
	a.search = search.New(db)
	a.cache = cache.New(db, a.cfg.CacheDir, a.cfg.ThumbnailDir, a.cfg.MaxCacheSizeMB)

	a.cfg.SetFirstRun(true)
	a.cfg.SetEnabledSources([]string{"wallhaven", "unsplash", "pexels"})
	a.cfg.Save(a.appDir)

	log.Printf("[app] database reset")
	return nil
}
