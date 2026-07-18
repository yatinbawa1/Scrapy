package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"wallpaper-chooser/internal/ai/aesthetic"
	"wallpaper-chooser/internal/ai/embeddings"
	"wallpaper-chooser/internal/ai/tags"
	"wallpaper-chooser/internal/cache"
	"wallpaper-chooser/internal/config"
	"wallpaper-chooser/internal/database"
	"wallpaper-chooser/internal/downloader"
	"wallpaper-chooser/internal/image/colors"
	"wallpaper-chooser/internal/image/duplicate"
	"wallpaper-chooser/internal/image/metadata"
	"wallpaper-chooser/internal/image/quality"
	"wallpaper-chooser/internal/scraper"
	"wallpaper-chooser/internal/search"
	"wallpaper-chooser/internal/search/semantic"
	"wallpaper-chooser/internal/search/similarity"
	"wallpaper-chooser/internal/thumbnail"
	"wallpaper-chooser/internal/workers/processing"
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
	embedder   embeddings.Embedder
	pool       *processing.Pool
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

	// AI analysis pipeline: CLIP embedder when available (build tag + model),
	// otherwise the dependency-free heuristic embedder. Both satisfy the
	// embeddings.Embedder interface.
	a.embedder = newEmbedder(a.cfg.ModelDir)
	a.pool = processing.New(a, 4)

	// If the active embedder's vector dimension differs from what was used to
	// analyze the library before, the stored embeddings are incompatible.
	// Clear them and re-analyze (preserving custom labels) so semantic search
	// stays correct after switching embedders.
	if prev, ok := a.db.GetSetting("embedding_dim"); ok {
		if prev != strconv.Itoa(a.embedder.Dim()) {
			if err := a.db.ClearAllEmbeddings(); err == nil {
				a.db.SetSetting("embedding_dim", strconv.Itoa(a.embedder.Dim()))
				go func() {
					time.Sleep(100 * time.Millisecond)
					if n := a.ReanalyzeAll(); n > 0 {
						log.Printf("[app] embedding dimension changed (%s -> %d); re-analyzing %d wallpapers", prev, a.embedder.Dim(), n)
					}
				}()
			}
		}
	} else {
		a.db.SetSetting("embedding_dim", strconv.Itoa(a.embedder.Dim()))
	}
	a.pool.Start()

	go a.thumbnailDownloader()

	// Regenerate thumbnails for any already-downloaded wallpapers whose stored
	// thumbnail is missing or degenerate (e.g. from the old resize bug that
	// produced 1x2 blobs). New downloads regenerate on completion, so this only
	// needs to fix pre-existing data.
	go a.regenerateDownloadedThumbnails()

	// Analyze the existing library in the background so AI features (semantic
	// search, collections, color, similar, duplicates) are populated without
	// requiring a manual "Analyze Library" pass.
	go func() {
		time.Sleep(2 * time.Second)
		if n := a.AnalyzeAll(); n > 0 {
			log.Printf("[app] queued %d unanalyzed wallpapers for AI analysis", n)
		}
	}()

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
				// thumbnails are always encoded as JPEG by the generator
				thumbPath := filepath.Join(a.cfg.ThumbnailDir, fmt.Sprintf("%d.jpg", w.ID))
					if err := a.thumbs.Generate(w.LocalPath, thumbPath); err == nil {
						a.db.UpdateThumbnailPath(w.ID, thumbPath)
						log.Printf("[app] generated thumbnail for %d", w.ID)
					}
				}
				runtime.EventsEmit(a.ctx, "wallpaper:downloaded", p)
				// Kick off background AI analysis (metadata, colors, embedding, etc.)
				go a.pool.Enqueue(p.WallpaperID)
			case "failed", "duplicate":
				runtime.EventsEmit(a.ctx, "download:failed", p)
			}
		}
	}()

	log.Printf("[app] started, data dir: %s", a.appDir)
}

func (a *App) shutdown(ctx context.Context) {
	a.pool.Stop(ctx)
	a.dl.Stop()
	if closer, ok := a.embedder.(interface{ Close() }); ok {
		closer.Close()
	}
	if a.db != nil {
		a.db.Close()
	}
}

// regenerateDownloadedThumbnails rebuilds the local thumbnail for every
// downloaded wallpaper straight from its full-resolution local file. It only
// overwrites thumbnails that are missing or suspiciously small (the degenerate
// 1x2 blobs), leaving good thumbnails untouched.
func (a *App) regenerateDownloadedThumbnails() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[app] regenerateDownloadedThumbnails recovered: %v", r)
		}
	}()

	const batch = 200
	for offset := 0; ; offset += batch {
		ws, err := a.db.GetDownloadedSorted(batch, offset, "latest")
		if err != nil {
			break
		}
		for _, w := range ws {
			if w.LocalPath == "" {
				continue
			}
			thumbPath := filepath.Join(a.cfg.ThumbnailDir, fmt.Sprintf("%d.jpg", w.ID))
			if info, statErr := os.Stat(thumbPath); statErr == nil && info.Size() > 8*1024 {
				continue // already a real thumbnail
			}
			if err := a.thumbs.Generate(w.LocalPath, thumbPath); err == nil {
				a.db.UpdateThumbnailPath(w.ID, thumbPath)
			}
		}
		if len(ws) < batch {
			break
		}
	}
	log.Printf("[app] thumbnail regeneration pass complete")
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

func (a *App) DismissOnboarding() {
	a.cfg.SetFirstRun(false)
	a.cfg.Save(a.appDir)
	log.Printf("[app] onboarding dismissed")
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
	a.db.DeleteMetadata(id)
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

// toItems converts DB embedding rows into shared search items.
func toItems(rows []database.EmbeddingRow) []search.Item {
	items := make([]search.Item, 0, len(rows))
	for _, r := range rows {
		if len(r.Embedding) == 0 {
			continue
		}
		text := strings.ToLower(r.Title + " " + r.Category + " " + r.SearchTerm + " " + strings.Join(r.Tags, " "))
		items = append(items, search.Item{ID: r.ID, Embedding: r.Embedding, Text: text, Brightness: r.Brightness})
	}
	return items
}

// Analyze runs the full AI analysis pipeline for a single wallpaper. It implements
// processing.Analyzer so it can be consumed by the background worker pool.
func (a *App) Analyze(id int64, custom []string) error {
	if a.db.HasMetadata(id) {
		return nil
	}
	// Preserve any user-defined custom labels already attached to this wallpaper
	// (e.g. when re-analyzed without an explicit custom set).
	if custom == nil {
		if m, gerr := a.db.GetMetadata(id); gerr == nil {
			custom = m.CustomLabels
		}
	}
	return a.analyze(id, custom)
}

// analyze performs the full analysis pipeline for a wallpaper, embedding
// `custom` (user-defined) labels alongside the auto-detected image concepts.
func (a *App) analyze(id int64, custom []string) error {
	w, err := a.db.GetWallpaper(id)
	if err != nil {
		return err
	}
	// Determine a source image: prefer the local file (or its thumbnail), and if
	// neither exists, fetch the remote thumbnail so scraped wallpapers can still
	// be analyzed.
	src := ""
	cleanup := func() {}
	if w.LocalPath != "" {
		src = w.LocalPath
		if w.ThumbnailPath != "" {
			if _, statErr := os.Stat(w.ThumbnailPath); statErr == nil {
				src = w.ThumbnailPath
			}
		}
	} else {
		remote := w.ThumbnailURL
		if remote == "" {
			remote = w.URL
		}
		if remote == "" {
			return fmt.Errorf("no image available for %d", id)
		}
		tmp, ferr := a.fetchTempImage(remote)
		if ferr != nil {
			return ferr
		}
		src = tmp
		cleanup = func() { os.Remove(tmp) }
	}
	defer cleanup()

	if _, statErr := os.Stat(src); statErr != nil {
		return fmt.Errorf("image not accessible for %d: %w", id, statErr)
	}
	img, _, derr := metadata.Decode(src)
	if derr != nil {
		return derr
	}
	info, _ := metadata.Extract(src)
	dom, _ := colors.DominantColors(img, 5)
	q := quality.Analyze(img)
	phash, _ := duplicate.Hash(img)
	b, c, s := q.Normalize()
	category := a.inferCategory(dom, b, w)
	autoLabels := embeddings.ImageConcepts(img, b, s)
	emb, _ := a.embedder.EmbedAnalysis(embeddings.AnalysisInput{
		Path:           w.LocalPath,
		Image:          img,
		DominantColors: dom,
		Brightness:     b,
		Contrast:       c,
		Sharpness:      s,
		Category:       category,
		Tags:           w.Tags,
		Title:          w.Title,
		SearchTerm:     w.SearchTerm,
		AutoLabels:     autoLabels,
		CustomLabels:   custom,
	})
	tagList := tags.Generate(tags.Input{
		DominantColors: dom,
		Brightness:     b,
		Category:       category,
		Title:          w.Title,
		SearchTerm:     w.SearchTerm,
		Existing:       w.Tags,
	})
	aes := aesthetic.Score(s, c, b)
	m := &database.Metadata{
		WallpaperID:    id,
		Width:          info.Width,
		Height:         info.Height,
		AspectRatio:    metadata.AspectRatio(info.Width, info.Height),
		FileSize:       info.Size,
		Format:         info.Format,
		DominantColors: dom,
		Brightness:     b,
		Contrast:       c,
		Sharpness:      s,
		Embedding:      emb,
		Tags:           tagList,
		Labels:         autoLabels,
		CustomLabels:   custom,
		Category:       category,
		AestheticScore: aes,
		PerceptualHash: phash,
	}
	if err := a.db.UpsertMetadata(m); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "wallpaper:analyzed", map[string]interface{}{"id": id})
	return nil
}

// AnalyzeWallpaper forces re-analysis (deletes existing metadata first) while
// preserving any user-defined custom labels.
func (a *App) AnalyzeWallpaper(id int64) error {
	var custom []string
	if m, gerr := a.db.GetMetadata(id); gerr == nil {
		custom = m.CustomLabels
	}
	if err := a.db.DeleteMetadata(id); err != nil {
		return err
	}
	return a.analyze(id, custom)
}

// GetWallpaperLabels returns the auto-detected image labels and any
// user-defined custom labels stored for a wallpaper.
func (a *App) GetWallpaperLabels(id int64) (map[string][]string, error) {
	m, err := a.db.GetMetadata(id)
	if err != nil {
		return nil, err
	}
	return map[string][]string{"labels": m.Labels, "customLabels": m.CustomLabels}, nil
}

// SetWallpaperLabels stores user-defined custom labels for a wallpaper and
// re-embeds it so those labels become searchable in the AI vector space.
func (a *App) SetWallpaperLabels(id int64, custom []string) error {
	m, err := a.db.GetMetadata(id)
	if err != nil {
		return fmt.Errorf("wallpaper %d must be analyzed first: %w", id, err)
	}
	seen := make(map[string]bool)
	dedup := make([]string, 0, len(custom))
	for _, l := range custom {
		l = strings.TrimSpace(strings.ToLower(l))
		if l == "" || seen[l] {
			continue
		}
		seen[l] = true
		dedup = append(dedup, l)
	}
	m.CustomLabels = dedup
	emb, _ := a.embedder.EmbedAnalysis(embeddings.AnalysisInput{
		Path:           "",
		DominantColors: m.DominantColors,
		Brightness:     m.Brightness,
		Contrast:       m.Contrast,
		Sharpness:      m.Sharpness,
		Category:       m.Category,
		Tags:           m.Tags,
		AutoLabels:     m.Labels,
		CustomLabels:   dedup,
	})
	m.Embedding = emb
	if err := a.db.UpsertMetadata(m); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "wallpaper:updated", map[string]interface{}{"id": id})
	return nil
}

// AnalysisStatus reports progress of the background analysis pipeline.
type AnalysisStatus struct {
	Submitted int64 `json:"submitted"`
	Done      int64 `json:"done"`
	Active    int64 `json:"active"`
	Paused    bool  `json:"paused"`
}

// AnalysisStats returns current analysis progress.
func (a *App) AnalysisStats() AnalysisStatus {
	s, d, act := a.pool.Stats()
	return AnalysisStatus{Submitted: s, Done: d, Active: act, Paused: a.pool.IsPaused()}
}

// PauseAnalysis halts the analysis pipeline (in-flight work finishes, then workers wait).
func (a *App) PauseAnalysis() {
	a.pool.Pause()
}

// ResumeAnalysis resumes a paused analysis pipeline.
func (a *App) ResumeAnalysis() {
	a.pool.Resume()
}

// fetchTempImage downloads a remote image into the cache dir and returns the
// local path. The caller is responsible for deleting it.
func (a *App) fetchTempImage(url string) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, url)
	}
	ext := ".jpg"
	switch {
	case strings.Contains(url, ".png"):
		ext = ".png"
	case strings.Contains(url, ".webp"):
		ext = ".webp"
	case strings.Contains(url, ".gif"):
		ext = ".gif"
	}
	tmp, err := os.CreateTemp(a.cfg.CacheDir, "analyze-*"+ext)
	if err != nil {
		return "", err
	}
	defer tmp.Close()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

// AnalyzeAll enqueues every downloaded wallpaper that has not been analyzed yet.
func (a *App) AnalyzeAll() int {
	ids, err := a.db.GetUndanalyzedIDs()
	if err != nil {
		return 0
	}
	for _, id := range ids {
		go a.pool.Enqueue(id)
	}
	return len(ids)
}

// ReanalyzeAll re-runs analysis for every wallpaper, preserving any user-defined
// custom labels. Used by the "Re-run AI Analysis" control so already-analyzed
// wallpapers are refreshed too (not just the unanalyzed ones).
func (a *App) ReanalyzeAll() int {
	ids, err := a.db.GetAllIDs()
	if err != nil {
		return 0
	}
	for _, id := range ids {
		// Preserve custom labels across the re-analysis.
		var custom []string
		if m, gerr := a.db.GetMetadata(id); gerr == nil {
			custom = m.CustomLabels
		}
		if err := a.db.DeleteMetadata(id); err != nil {
			continue
		}
		go a.pool.EnqueueWith(id, custom)
	}
	return len(ids)
}

func (a *App) inferCategory(dom []string, bright float64, w *database.Wallpaper) string {
	if c := embeddings.ConceptOf(w.Title + " " + w.SearchTerm + " " + strings.Join(w.Tags, " ")); c != "" {
		return c
	}
	if bright < 0.25 {
		return "dark"
	}
	if bright > 0.75 {
		return "light"
	}
	return "general"
}

func (a *App) wallpapersByRank(res []search.Result) []database.Wallpaper {
	ids := make([]int64, len(res))
	order := make(map[int64]int, len(res))
	for i, r := range res {
		ids[i] = r.ID
		order[r.ID] = i
	}
	ws, _ := a.db.GetWallpapersByIDs(ids)
	sort.SliceStable(ws, func(i, j int) bool { return order[ws[i].ID] < order[ws[j].ID] })
	return ws
}

func (a *App) SemanticSearch(query string, limit int) []database.Wallpaper {
	if limit <= 0 {
		limit = 60
	}
	rows, err := a.db.GetAllEmbeddingRows()
	if err != nil {
		return nil
	}
	items := toItems(rows)
	res, err := semantic.RankText(a.embedder, items, query, limit)
	if err != nil {
		return nil
	}

	// Hybrid boost: a user tag or auto label that exactly matches a query token
	// must surface even when the concept isn't in the embedding vocabulary (e.g.
	// tagging a wallpaper "arch" and searching "arch"). This is what makes the
	// tagging feature actually useful.
	qTokens := tokenizeQuery(query)
	if len(qTokens) > 0 {
		scores := make(map[int64]float64, len(res))
		for _, r := range res {
			scores[r.ID] = r.Score
		}
		for _, row := range rows {
			labelText := strings.ToLower(strings.Join(row.CustomLabels, " ") + " " + strings.Join(row.Tags, " "))
			for _, t := range qTokens {
				if t == "" {
					continue
				}
				if strings.Contains(labelText, t) {
					if s, ok := scores[row.ID]; !ok || s < 1.5 {
						scores[row.ID] = 1.5
					}
				}
			}
		}
		merged := make([]search.Result, 0, len(scores))
		for id, sc := range scores {
			merged = append(merged, search.Result{ID: id, Score: sc})
		}
		sortResults(merged)
		if len(merged) > limit {
			merged = merged[:limit]
		}
		return a.wallpapersByRank(merged)
	}

	return a.wallpapersByRank(res)
}

// tokenizeQuery splits a natural-language query into lowercase tokens.
func tokenizeQuery(q string) []string {
	parts := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return r == ' ' || r == ',' || r == '-' || r == '.' || r == '/' || r == ':'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(p, ".,")
		if len(p) > 1 {
			out = append(out, p)
		}
	}
	return out
}

// sortResults orders search results by descending score (stable insertion sort).
func sortResults(r []search.Result) {
	for i := 1; i < len(r); i++ {
		for j := i; j > 0 && r[j].Score > r[j-1].Score; j-- {
			r[j], r[j-1] = r[j-1], r[j]
		}
	}
}

func (a *App) FindSimilar(id int64, limit int) []database.Wallpaper {
	if limit <= 0 {
		limit = 60
	}
	m, err := a.db.GetMetadata(id)
	if err != nil || len(m.Embedding) == 0 {
		return nil
	}
	rows, err := a.db.GetAllEmbeddingRows()
	if err != nil {
		return nil
	}
	items := toItems(rows)
	res := similarity.Rank(m.Embedding, items, id, limit)
	return a.wallpapersByRank(res)
}

type Collection struct {
	Name      string  `json:"name"`
	Count     int     `json:"count"`
	SampleIDs []int64 `json:"sampleIds"`
}

func (a *App) GetCollections() []Collection {
	rows, err := a.db.GetAllEmbeddingRows()
	if err != nil {
		return nil
	}
	catCounts := map[string][]int64{}
	dark, light := []int64{}, []int64{}
	for _, r := range rows {
		if r.Category != "" && r.Category != "general" {
			catCounts[r.Category] = append(catCounts[r.Category], r.ID)
		}
		if r.Brightness < 0.25 {
			dark = append(dark, r.ID)
		}
		if r.Brightness > 0.75 {
			light = append(light, r.ID)
		}
	}
	var cols []Collection
	add := func(name string, ids []int64) {
		if len(ids) == 0 {
			return
		}
		s := ids
		if len(s) > 8 {
			s = s[:8]
		}
		cols = append(cols, Collection{Name: name, Count: len(ids), SampleIDs: s})
	}
	add("Dark", dark)
	add("Light", light)
	for name, ids := range catCounts {
		add(name, ids)
	}
	// Group by dominant color names.
	colorCounts := map[string][]int64{}
	for _, r := range rows {
		for _, hex := range r.DominantColors {
			name := colors.ColorName(hex)
			if name != "" {
				colorCounts[name] = append(colorCounts[name], r.ID)
			}
		}
	}
	for name, ids := range colorCounts {
		add(name, ids)
	}
	sort.Slice(cols, func(i, j int) bool { return cols[i].Count > cols[j].Count })
	return cols
}

func (a *App) CollectionWallpapers(name string) []database.Wallpaper {
	rows, err := a.db.GetAllEmbeddingRows()
	if err != nil {
		return nil
	}
	var ids []int64
	lname := strings.ToLower(name)
	switch lname {
	case "dark":
		for _, r := range rows {
			if r.Brightness < 0.25 {
				ids = append(ids, r.ID)
			}
		}
	case "light":
		for _, r := range rows {
			if r.Brightness > 0.75 {
				ids = append(ids, r.ID)
			}
		}
	default:
		for _, r := range rows {
			if strings.ToLower(r.Category) == lname {
				ids = append(ids, r.ID)
				continue
			}
			for _, hex := range r.DominantColors {
				if strings.ToLower(colors.ColorName(hex)) == lname {
					ids = append(ids, r.ID)
					break
				}
			}
		}
	}
	ws, _ := a.db.GetWallpapersByIDs(ids)
	return ws
}

func (a *App) SearchByColor(hex string, limit int) []database.Wallpaper {
	if limit <= 0 {
		limit = 60
	}
	rows, err := a.db.GetAllEmbeddingRows()
	if err != nil {
		return nil
	}
	type scored struct {
		id int64
		d  float64
	}
	var list []scored
	for _, r := range rows {
		best := -1.0
		for _, h := range r.DominantColors {
			d := colors.NearestColorDistance(h, hex)
			if best < 0 || d < best {
				best = d
			}
		}
		if best < 0 {
			continue
		}
		list = append(list, scored{id: r.ID, d: best})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].d < list[j].d })
	if len(list) > limit {
		list = list[:limit]
	}
	ids := make([]int64, len(list))
	order := make(map[int64]int, len(list))
	for i, s := range list {
		ids[i] = s.id
		order[s.id] = i
	}
	ws, _ := a.db.GetWallpapersByIDs(ids)
	sort.SliceStable(ws, func(i, j int) bool { return order[ws[i].ID] < order[ws[j].ID] })
	return ws
}

type DuplicateGroup struct {
	Hash string  `json:"hash"`
	IDs  []int64 `json:"ids"`
}

// duplicateGroups clusters wallpapers by perceptual-hash similarity (Hamming
// distance <= 10) and returns only groups with more than one member.
func (a *App) duplicateGroups() ([][]int64, error) {
	rows, err := a.db.GetAllPerceptualHashes()
	if err != nil {
		return nil, err
	}
	n := len(rows)
	if n == 0 {
		return nil, nil
	}
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			x = parent[x]
		}
		return x
	}
	union := func(x, y int) {
		rx, ry := find(x), find(y)
		if rx != ry {
			parent[rx] = ry
		}
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if duplicate.HammingDistance(rows[i].Hash, rows[j].Hash) <= 10 {
				union(i, j)
			}
		}
	}
	clusters := map[int][]int64{}
	for i := 0; i < n; i++ {
		clusters[find(i)] = append(clusters[find(i)], rows[i].ID)
	}
	var groups [][]int64
	for _, ids := range clusters {
		if len(ids) > 1 {
			groups = append(groups, ids)
		}
	}
	return groups, nil
}

func (a *App) GetDuplicates() []DuplicateGroup {
	groups, err := a.duplicateGroups()
	if err != nil || len(groups) == 0 {
		return nil
	}
	out := make([]DuplicateGroup, 0, len(groups))
	for _, ids := range groups {
		out = append(out, DuplicateGroup{IDs: ids})
	}
	return out
}

// pickBest returns the id within a duplicate group that should be kept: prefer a
// downloaded copy, then the largest file (most detail), then the lowest id.
func (a *App) pickBest(ids []int64) int64 {
	best := ids[0]
	bestScore := -1.0
	for _, id := range ids {
		w, err := a.db.GetWallpaper(id)
		if err != nil {
			continue
		}
		score := 0.0
		if w.Status == "downloaded" {
			score += 2.0
		}
		score += float64(w.Filesize) / 1e7
		if score > bestScore {
			bestScore = score
			best = id
		}
	}
	return best
}

// DeleteDuplicates removes all but the best wallpaper from every duplicate
// group. Returns the number of wallpapers deleted.
func (a *App) DeleteDuplicates() (int, error) {
	groups, err := a.duplicateGroups()
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, g := range groups {
		keep := a.pickBest(g)
		for _, id := range g {
			if id == keep {
				continue
			}
			if err := a.DeleteWallpaper(id); err == nil {
				deleted++
			}
		}
	}
	runtime.EventsEmit(a.ctx, "wallpaper:deleted", nil)
	runtime.EventsEmit(a.ctx, "storage:updated", nil)
	return deleted, nil
}

// GetWallpapersByIDs returns wallpapers for the given ids (used for duplicate review, etc.).
func (a *App) GetWallpapersByIDs(ids []int64) []database.Wallpaper {
	ws, _ := a.db.GetWallpapersByIDs(ids)
	return ws
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
