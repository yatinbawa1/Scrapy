package database

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

type Wallpaper struct {
	ID            int64     `json:"id"`
	URL           string    `json:"url"`
	LocalPath     string    `json:"localPath"`
	ThumbnailURL  string    `json:"thumbnailUrl"`
	ThumbnailPath string    `json:"thumbnailPath"`
	Width         int       `json:"width"`
	Height        int       `json:"height"`
	Filesize      int64     `json:"filesize"`
	Source        string    `json:"source"`
	SearchTerm    string    `json:"searchTerm"`
	Hash          string    `json:"hash"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	Tags          []string  `json:"tags"`
	IsFavorite    bool      `json:"isFavorite"`
	Status        string    `json:"status"`
	Brightness    float64   `json:"brightness"`
	CreatedAt     time.Time `json:"createdAt"`
}

type Source struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
	Enabled bool   `json:"enabled"`
}

type SearchResult struct {
	Wallpapers []Wallpaper `json:"wallpapers"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"pageSize"`
}

// Metadata holds the AI/visual analysis of a wallpaper. Stored separately from
// the core wallpapers table so the analysis pipeline can evolve independently.
type Metadata struct {
	WallpaperID   int64     `json:"wallpaperId"`
	Width         int       `json:"width"`
	Height        int       `json:"height"`
	AspectRatio   float64   `json:"aspectRatio"`
	FileSize      int64     `json:"fileSize"`
	Format        string    `json:"format"`
	DominantColors []string `json:"dominantColors"`
	Brightness    float64   `json:"brightness"`
	Contrast      float64   `json:"contrast"`
	Sharpness     float64   `json:"sharpness"`
	Embedding     []float32 `json:"embedding"`
	Tags          []string  `json:"tags"`
	Labels        []string  `json:"labels"`
	CustomLabels  []string  `json:"customLabels"`
	Category      string    `json:"category"`
	AestheticScore float64  `json:"aestheticScore"`
	PerceptualHash string   `json:"perceptualHash"`
	CreatedAt     time.Time `json:"createdAt"`
}

// EmbeddingRow is a lightweight projection used for similarity/semantic search.
type EmbeddingRow struct {
	ID            int64
	Embedding     []float32
	DominantColors []string
	Brightness    float64
	Category      string
	Tags          []string
	CustomLabels  []string
	Title         string
	SearchTerm    string
	Source        string
}

const selectCols = `id, url, local_path, thumbnail_url, thumbnail_path, width, height, filesize, source, search_term, hash, title, description, is_favorite, status, brightness, created_at`

func New(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=10000&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	conn.SetMaxOpenConns(5)
	conn.SetMaxIdleConns(2)

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS sources (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			base_url TEXT NOT NULL,
			enabled INTEGER DEFAULT 1
		)`,
		`CREATE TABLE IF NOT EXISTS wallpapers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url TEXT UNIQUE NOT NULL,
			local_path TEXT DEFAULT '',
			thumbnail_url TEXT DEFAULT '',
			thumbnail_path TEXT DEFAULT '',
			width INTEGER DEFAULT 0,
			height INTEGER DEFAULT 0,
			filesize INTEGER DEFAULT 0,
			source TEXT NOT NULL,
			search_term TEXT DEFAULT '',
			hash TEXT DEFAULT '',
			title TEXT DEFAULT '',
			description TEXT DEFAULT '',
			is_favorite INTEGER DEFAULT 0,
			status TEXT DEFAULT 'scraped',
			brightness REAL DEFAULT -1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS tags (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS wallpaper_tags (
			wallpaper_id INTEGER NOT NULL,
			tag_id INTEGER NOT NULL,
			PRIMARY KEY (wallpaper_id, tag_id),
			FOREIGN KEY (wallpaper_id) REFERENCES wallpapers(id) ON DELETE CASCADE,
			FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_wallpapers_source ON wallpapers(source)`,
		`CREATE INDEX IF NOT EXISTS idx_wallpapers_hash ON wallpapers(hash)`,
		`CREATE INDEX IF NOT EXISTS idx_wallpapers_status ON wallpapers(status)`,
		`CREATE INDEX IF NOT EXISTS idx_wallpapers_favorite ON wallpapers(is_favorite)`,
		`CREATE INDEX IF NOT EXISTS idx_wallpapers_search_term ON wallpapers(search_term)`,
		`CREATE INDEX IF NOT EXISTS idx_tags_name ON tags(name)`,
		`CREATE TABLE IF NOT EXISTS wallpaper_metadata (
			wallpaper_id INTEGER PRIMARY KEY,
			width INTEGER DEFAULT 0,
			height INTEGER DEFAULT 0,
			aspect_ratio REAL DEFAULT 0,
			file_size INTEGER DEFAULT 0,
			format TEXT DEFAULT '',
			dominant_colors TEXT DEFAULT '[]',
			brightness REAL DEFAULT -1,
			contrast REAL DEFAULT -1,
			sharpness REAL DEFAULT -1,
			embedding BLOB,
			tags TEXT DEFAULT '[]',
			category TEXT DEFAULT '',
			aesthetic_score REAL DEFAULT -1,
			perceptual_hash TEXT DEFAULT '',
			labels TEXT DEFAULT '[]',
			custom_labels TEXT DEFAULT '[]',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	}

	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			return fmt.Errorf("exec %q: %w", q[:40], err)
		}
	}

	// Migrate existing databases that predate the labels columns.
	for _, col := range []string{"labels", "custom_labels"} {
		_, err := db.conn.Exec(fmt.Sprintf("ALTER TABLE wallpaper_metadata ADD COLUMN %s TEXT DEFAULT '[]'", col))
		if err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("migrate column %s: %w", col, err)
		}
	}

	db.conn.Exec(`ALTER TABLE wallpapers ADD COLUMN thumbnail_url TEXT DEFAULT ''`)
	db.conn.Exec(`ALTER TABLE wallpapers ADD COLUMN search_term TEXT DEFAULT ''`)
	db.conn.Exec(`ALTER TABLE wallpapers ADD COLUMN brightness REAL DEFAULT -1`)
	db.conn.Exec(`ALTER TABLE wallpapers ADD COLUMN thumb_attempts INTEGER DEFAULT 0`)

	db.conn.Exec(`UPDATE wallpapers SET status = 'scraped' WHERE status = 'pending'`)

	defaultSources := []struct{ name, url string }{
		{"unsplash", "https://unsplash.com"},
		{"pexels", "https://pexels.com"},
		{"wallhaven", "https://wallhaven.cc"},
	}
	for _, s := range defaultSources {
		db.conn.Exec(`INSERT OR IGNORE INTO sources (name, base_url, enabled) VALUES (?, ?, 1)`, s.name, s.url)
	}

	return nil
}

func (db *DB) InsertWallpaper(w *Wallpaper) (int64, error) {
	result, err := db.conn.Exec(`
		INSERT OR IGNORE INTO wallpapers (url, local_path, thumbnail_url, thumbnail_path, width, height, filesize, source, search_term, hash, title, description, status, brightness)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.URL, w.LocalPath, w.ThumbnailURL, w.ThumbnailPath, w.Width, w.Height, w.Filesize, w.Source, w.SearchTerm, w.Hash, w.Title, w.Description, w.Status, w.Brightness)
	if err != nil {
		return 0, err
	}
	id, _ := result.LastInsertId()

	if id > 0 && len(w.Tags) > 0 {
		for _, tag := range w.Tags {
			tagID, _ := db.getOrCreateTag(tag)
			db.conn.Exec(`INSERT OR IGNORE INTO wallpaper_tags (wallpaper_id, tag_id) VALUES (?, ?)`, id, tagID)
		}
	}

	return id, nil
}

func (db *DB) getOrCreateTag(name string) (int64, error) {
	var id int64
	err := db.conn.QueryRow(`SELECT id FROM tags WHERE name = ?`, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	result, err := db.conn.Exec(`INSERT INTO tags (name) VALUES (?)`, name)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (db *DB) GetWallpaper(id int64) (*Wallpaper, error) {
	w := &Wallpaper{}
	err := db.conn.QueryRow(`SELECT `+selectCols+` FROM wallpapers WHERE id = ?`, id).Scan(
		&w.ID, &w.URL, &w.LocalPath, &w.ThumbnailURL, &w.ThumbnailPath, &w.Width, &w.Height, &w.Filesize, &w.Source, &w.SearchTerm, &w.Hash, &w.Title, &w.Description, &w.IsFavorite, &w.Status, &w.Brightness, &w.CreatedAt)
	if err != nil {
		return nil, err
	}

	rows, err := db.conn.Query(`SELECT t.name FROM tags t JOIN wallpaper_tags wt ON t.id = wt.tag_id WHERE wt.wallpaper_id = ?`, w.ID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var tag string
			rows.Scan(&tag)
			w.Tags = append(w.Tags, tag)
		}
	}

	return w, nil
}

func (db *DB) ExistsByURL(url string) bool {
	var id int64
	err := db.conn.QueryRow(`SELECT id FROM wallpapers WHERE url = ?`, url).Scan(&id)
	return err == nil
}

func (db *DB) ExistsByHash(hash string) bool {
	var id int64
	err := db.conn.QueryRow(`SELECT id FROM wallpapers WHERE hash = ? AND hash != ''`, hash).Scan(&id)
	return err == nil
}

func (db *DB) UpdateLocalPath(id int64, localPath string) error {
	_, err := db.conn.Exec(`UPDATE wallpapers SET local_path = ?, status = 'downloaded' WHERE id = ?`,
		localPath, id)
	return err
}

func (db *DB) UpdateThumbnailPath(id int64, thumbnailPath string) error {
	_, err := db.conn.Exec(`UPDATE wallpapers SET thumbnail_path = ?, thumb_attempts = 0 WHERE id = ?`, thumbnailPath, id)
	return err
}

func (db *DB) IncrementThumbAttempts(id int64) error {
	_, err := db.conn.Exec(`UPDATE wallpapers SET thumb_attempts = thumb_attempts + 1 WHERE id = ?`, id)
	return err
}

func (db *DB) UpdateStatus(id int64, status string) error {
	_, err := db.conn.Exec(`UPDATE wallpapers SET status = ? WHERE id = ?`, status, id)
	return err
}

func (db *DB) UpdateHash(id int64, hash string) error {
	_, err := db.conn.Exec(`UPDATE wallpapers SET hash = ? WHERE id = ?`, hash, id)
	return err
}

func (db *DB) UpdateBrightness(id int64, brightness float64) error {
	_, err := db.conn.Exec(`UPDATE wallpapers SET brightness = ? WHERE id = ?`, brightness, id)
	return err
}

func (db *DB) SetFavorite(id int64, fav bool) error {
	v := 0
	if fav {
		v = 1
	}
	_, err := db.conn.Exec(`UPDATE wallpapers SET is_favorite = ? WHERE id = ?`, v, id)
	return err
}

func (db *DB) GetScrapedWithoutThumbnail(limit int) ([]Wallpaper, error) {
	return db.queryWallpapers(`SELECT `+selectCols+` FROM wallpapers WHERE status = 'scraped' AND thumbnail_url != '' AND thumbnail_path = '' AND thumb_attempts < 3 ORDER BY created_at ASC LIMIT ?`, limit)
}

func (db *DB) GetDownloaded(limit, offset int) ([]Wallpaper, error) {
	return db.GetDownloadedSorted(limit, offset, "latest")
}

func (db *DB) GetDownloadedSorted(limit, offset int, sortBy string) ([]Wallpaper, error) {
	order := "created_at DESC"
	switch sortBy {
	case "source":
		order = "source ASC, created_at DESC"
	case "dark":
		order = "brightness ASC, created_at DESC"
	case "light":
		order = "brightness DESC, created_at DESC"
	case "latest":
		order = "created_at DESC"
	}
	return db.queryWallpapers(fmt.Sprintf(`SELECT `+selectCols+` FROM wallpapers WHERE status = 'downloaded' ORDER BY %s LIMIT ? OFFSET ?`, order), limit, offset)
}

func (db *DB) GetAllSorted(limit, offset int, sortBy string) ([]Wallpaper, int, error) {
	return db.GetAllSortedFiltered(limit, offset, sortBy, "", "", "")
}

func (db *DB) GetAllSortedFiltered(limit, offset int, sortBy string, searchTerm string, query string, source string) ([]Wallpaper, int, error) {
	where := `WHERE status IN ('scraped', 'downloaded')`
	var args []interface{}
	if searchTerm != "" {
		where += ` AND search_term = ?`
		args = append(args, searchTerm)
	}
	if query != "" {
		where += ` AND (title LIKE ? OR description LIKE ? OR url LIKE ? OR search_term LIKE ?)`
		q := "%" + query + "%"
		args = append(args, q, q, q, q)
	}
	if source != "" {
		where += ` AND source = ?`
		args = append(args, source)
	}

	var total int
	db.conn.QueryRow(`SELECT COUNT(*) FROM wallpapers `+where, args...).Scan(&total)

	if total == 0 {
		return nil, 0, nil
	}

	if sortBy == "random" || sortBy == "" {
		return db.getPaginatedRandom(limit, offset, total, where, args)
	}

	order := "created_at DESC"
	switch sortBy {
	case "source":
		order = "source ASC, id ASC"
	case "dark":
		order = "brightness ASC, id ASC"
	case "light":
		order = "brightness DESC, id ASC"
	}

	args = append(args, limit, offset)
	w, err := db.queryWallpapers(fmt.Sprintf(`SELECT `+selectCols+` FROM wallpapers %s ORDER BY %s LIMIT ? OFFSET ?`, where, order), args...)
	return w, total, err
}

func (db *DB) getPaginatedRandom(limit, offset, total int, where string, baseArgs []interface{}) ([]Wallpaper, int, error) {
	if total == 0 {
		return nil, 0, nil
	}

	args := append([]interface{}{}, baseArgs...)
	args = append(args, limit, offset)

	q := fmt.Sprintf(`SELECT `+selectCols+` FROM wallpapers %s ORDER BY RANDOM() LIMIT ? OFFSET ?`, where)
	w, err := db.queryWallpapers(q, args...)
	return w, total, err
}

func (db *DB) GetFavorites(limit, offset int) ([]Wallpaper, error) {
	return db.queryWallpapers(`SELECT `+selectCols+` FROM wallpapers WHERE is_favorite = 1 ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
}

func (db *DB) Search(query string, source string, minW, minH int, limit, offset int) (SearchResult, error) {
	where := `WHERE status = 'downloaded'`
	args := []interface{}{}

	if query != "" {
		where += ` AND (title LIKE ? OR description LIKE ? OR url LIKE ? OR search_term LIKE ?)`
		q := "%" + query + "%"
		args = append(args, q, q, q, q)
	}
	if source != "" {
		where += ` AND source = ?`
		args = append(args, source)
	}
	if minW > 0 {
		where += ` AND width >= ?`
		args = append(args, minW)
	}
	if minH > 0 {
		where += ` AND height >= ?`
		args = append(args, minH)
	}

	countQ := `SELECT COUNT(*) FROM wallpapers ` + where
	var total int
	db.conn.QueryRow(countQ, args...).Scan(&total)

	q := fmt.Sprintf(`SELECT `+selectCols+` FROM wallpapers %s ORDER BY created_at DESC LIMIT ? OFFSET ?`, where)
	args = append(args, limit, offset)

	wallpapers, err := db.queryWallpapers(q, args...)
	if err != nil {
		return SearchResult{}, err
	}

	return SearchResult{
		Wallpapers: wallpapers,
		Total:      total,
		Page:       offset/limit + 1,
		PageSize:   limit,
	}, nil
}

func (db *DB) GetCategoryStats() ([]map[string]interface{}, error) {
	rows, err := db.conn.Query(`
		SELECT search_term, COUNT(*) as count
		FROM wallpapers
		WHERE search_term != '' AND status IN ('scraped', 'downloaded')
		GROUP BY search_term
		ORDER BY count DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []map[string]interface{}
	for rows.Next() {
		var term string
		var count int
		rows.Scan(&term, &count)
		cats = append(cats, map[string]interface{}{
			"term":  term,
			"count": count,
		})
	}
	return cats, nil
}

func (db *DB) GetSourceStats() ([]map[string]interface{}, error) {
	rows, err := db.conn.Query(`
		SELECT source, COUNT(*) as count,
			SUM(CASE WHEN status = 'downloaded' THEN 1 ELSE 0 END) as downloaded,
			SUM(CASE WHEN is_favorite = 1 THEN 1 ELSE 0 END) as favorites
		FROM wallpapers
		GROUP BY source
		ORDER BY count DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sources []map[string]interface{}
	for rows.Next() {
		var source string
		var count, downloaded, favorites int
		rows.Scan(&source, &count, &downloaded, &favorites)
		sources = append(sources, map[string]interface{}{
			"name":       source,
			"count":      count,
			"downloaded": downloaded,
			"favorites":  favorites,
		})
	}
	return sources, nil
}

func (db *DB) GetSources() ([]Source, error) {
	rows, err := db.conn.Query(`SELECT id, name, base_url, enabled FROM sources`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sources []Source
	for rows.Next() {
		var s Source
		rows.Scan(&s.ID, &s.Name, &s.BaseURL, &s.Enabled)
		sources = append(sources, s)
	}
	return sources, nil
}

func (db *DB) GetStats() (map[string]interface{}, error) {
	stats := map[string]interface{}{}

	var total int
	db.conn.QueryRow(`SELECT COUNT(*) FROM wallpapers`).Scan(&total)
	stats["total"] = total

	var downloaded int
	db.conn.QueryRow(`SELECT COUNT(*) FROM wallpapers WHERE status = 'downloaded'`).Scan(&downloaded)
	stats["downloaded"] = downloaded

	var scraped int
	db.conn.QueryRow(`SELECT COUNT(*) FROM wallpapers WHERE status = 'scraped'`).Scan(&scraped)
	stats["scraped"] = scraped

	var pending int
	db.conn.QueryRow(`SELECT COUNT(*) FROM wallpapers WHERE status = 'pending'`).Scan(&pending)
	stats["pending"] = pending

	var favorites int
	db.conn.QueryRow(`SELECT COUNT(*) FROM wallpapers WHERE is_favorite = 1`).Scan(&favorites)
	stats["favorites"] = favorites

	return stats, nil
}

func (db *DB) DeleteWallpaper(id int64) error {
	_, err := db.conn.Exec(`DELETE FROM wallpapers WHERE id = ?`, id)
	return err
}

func (db *DB) DeleteAllWallpapers() error {
	rows, err := db.conn.Query(`SELECT local_path, thumbnail_path FROM wallpapers`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var localPath, thumbPath sql.NullString
			if err := rows.Scan(&localPath, &thumbPath); err == nil {
				if localPath.Valid && localPath.String != "" {
					os.Remove(localPath.String)
				}
				if thumbPath.Valid && thumbPath.String != "" {
					os.Remove(thumbPath.String)
				}
			}
		}
	}

	if _, err := db.conn.Exec(`DELETE FROM wallpapers`); err != nil {
		return err
	}
	db.conn.Exec(`DELETE FROM wallpaper_tags`)
	db.conn.Exec(`DELETE FROM tags`)
	db.conn.Exec(`DELETE FROM sqlite_sequence`)
	return nil
}

// --- Metadata / AI analysis repository ---

func floatsToBytes(f []float32) []byte {
	b := make([]byte, len(f)*4)
	for i, v := range f {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(v))
	}
	return b
}

func bytesToFloats(b []byte) []float32 {
	if len(b) == 0 || len(b)%4 != 0 {
		return nil
	}
	n := len(b) / 4
	f := make([]float32, n)
	for i := 0; i < n; i++ {
		f[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return f
}

func (db *DB) UpsertMetadata(m *Metadata) error {
	colors, _ := json.Marshal(m.DominantColors)
	tags, _ := json.Marshal(m.Tags)
	labels, _ := json.Marshal(m.Labels)
	custom, _ := json.Marshal(m.CustomLabels)
	_, err := db.conn.Exec(`
		INSERT INTO wallpaper_metadata
		(wallpaper_id, width, height, aspect_ratio, file_size, format, dominant_colors, brightness, contrast, sharpness, embedding, tags, labels, custom_labels, category, aesthetic_score, perceptual_hash)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(wallpaper_id) DO UPDATE SET
		width=excluded.width, height=excluded.height, aspect_ratio=excluded.aspect_ratio,
		file_size=excluded.file_size, format=excluded.format, dominant_colors=excluded.dominant_colors,
		brightness=excluded.brightness, contrast=excluded.contrast, sharpness=excluded.sharpness,
		embedding=excluded.embedding, tags=excluded.tags, labels=excluded.labels,
		custom_labels=excluded.custom_labels, category=excluded.category,
		aesthetic_score=excluded.aesthetic_score, perceptual_hash=excluded.perceptual_hash`,
		m.WallpaperID, m.Width, m.Height, m.AspectRatio, m.FileSize, m.Format, string(colors),
		m.Brightness, m.Contrast, m.Sharpness, floatsToBytes(m.Embedding), string(tags),
		string(labels), string(custom), m.Category, m.AestheticScore, m.PerceptualHash)
	return err
}

func (db *DB) GetMetadata(id int64) (*Metadata, error) {
	var m Metadata
	var colorsJSON, tagsJSON, labelsJSON, customJSON, emb []byte
	var width, height int
	var ar float64
	var fileSize int64
	var format string
	var brightness, contrast, sharpness, aes float64
	var category, phash string
	err := db.conn.QueryRow(`
		SELECT width, height, aspect_ratio, file_size, format, dominant_colors, brightness, contrast, sharpness, embedding, tags, labels, custom_labels, category, aesthetic_score, perceptual_hash
		FROM wallpaper_metadata WHERE wallpaper_id = ?`, id).
		Scan(&width, &height, &ar, &fileSize, &format, &colorsJSON, &brightness, &contrast, &sharpness, &emb, &tagsJSON, &labelsJSON, &customJSON, &category, &aes, &phash)
	if err != nil {
		return nil, err
	}
	m.WallpaperID = id
	m.Width = width
	m.Height = height
	m.AspectRatio = ar
	m.FileSize = fileSize
	m.Format = format
	json.Unmarshal(colorsJSON, &m.DominantColors)
	json.Unmarshal(tagsJSON, &m.Tags)
	json.Unmarshal(labelsJSON, &m.Labels)
	json.Unmarshal(customJSON, &m.CustomLabels)
	m.Brightness = brightness
	m.Contrast = contrast
	m.Sharpness = sharpness
	m.Embedding = bytesToFloats(emb)
	m.Category = category
	m.AestheticScore = aes
	m.PerceptualHash = phash
	return &m, nil
}

func (db *DB) HasMetadata(id int64) bool {
	var n int
	db.conn.QueryRow(`SELECT COUNT(*) FROM wallpaper_metadata WHERE wallpaper_id = ?`, id).Scan(&n)
	return n > 0
}

func (db *DB) GetAllEmbeddingRows() ([]EmbeddingRow, error) {
	rows, err := db.conn.Query(`
		SELECT wm.wallpaper_id, wm.embedding, wm.dominant_colors, wm.brightness, wm.category, wm.tags, wm.custom_labels,
		       w.title, w.search_term, w.source
		FROM wallpaper_metadata wm
		JOIN wallpapers w ON w.id = wm.wallpaper_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EmbeddingRow
	for rows.Next() {
		var r EmbeddingRow
		var emb []byte
		var colorsJSON, tagsJSON, customJSON []byte
		if err := rows.Scan(&r.ID, &emb, &colorsJSON, &r.Brightness, &r.Category, &tagsJSON, &customJSON, &r.Title, &r.SearchTerm, &r.Source); err != nil {
			continue
		}
		r.Embedding = bytesToFloats(emb)
		json.Unmarshal(colorsJSON, &r.DominantColors)
		json.Unmarshal(tagsJSON, &r.Tags)
		json.Unmarshal(customJSON, &r.CustomLabels)
		out = append(out, r)
	}
	return out, nil
}

func (db *DB) GetAllPerceptualHashes() ([]struct {
	ID   int64
	Hash string
}, error) {
	rows, err := db.conn.Query(`SELECT wallpaper_id, perceptual_hash FROM wallpaper_metadata WHERE perceptual_hash != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		ID   int64
		Hash string
	}
	for rows.Next() {
		var r struct {
			ID   int64
			Hash string
		}
		if err := rows.Scan(&r.ID, &r.Hash); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func (db *DB) GetWallpapersByIDs(ids []int64) ([]Wallpaper, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	return db.queryWallpapers(fmt.Sprintf(`SELECT `+selectCols+` FROM wallpapers WHERE id IN (%s)`, strings.Join(placeholders, ",")), args...)
}

func (db *DB) DeleteMetadata(id int64) error {
	_, err := db.conn.Exec(`DELETE FROM wallpaper_metadata WHERE wallpaper_id = ?`, id)
	return err
}

// GetUndanalyzedIDs returns ids of wallpapers that have no metadata yet. This
// covers both downloaded wallpapers (local file) and scraped ones (remote
// image is fetched during analysis).
func (db *DB) GetAllIDs() ([]int64, error) {
	rows, err := db.conn.Query(`SELECT id FROM wallpapers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (db *DB) GetUndanalyzedIDs() ([]int64, error) {
	rows, err := db.conn.Query(`SELECT id FROM wallpapers WHERE id NOT IN (SELECT wallpaper_id FROM wallpaper_metadata)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// GetSetting returns a stored key/value setting, or ("", false) if unset.
func (db *DB) GetSetting(key string) (string, bool) {
	var v string
	err := db.conn.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err != nil {
		return "", false
	}
	return v, true
}

// SetSetting stores (upserts) a key/value setting.
func (db *DB) SetSetting(key, value string) error {
	_, err := db.conn.Exec(`
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
}

// ClearAllEmbeddings nulls out every stored embedding. Used when the active
// embedder changes dimension (e.g. switching between the heuristic and CLIP
// embedders) so stale, incompatible vectors don't poison semantic search.
func (db *DB) ClearAllEmbeddings() error {
	_, err := db.conn.Exec(`UPDATE wallpaper_metadata SET embedding = NULL`)
	return err
}

func (db *DB) queryWallpapers(query string, args ...interface{}) ([]Wallpaper, error) {
	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var wallpapers []Wallpaper
	for rows.Next() {
		var w Wallpaper
		err := rows.Scan(&w.ID, &w.URL, &w.LocalPath, &w.ThumbnailURL, &w.ThumbnailPath, &w.Width, &w.Height, &w.Filesize, &w.Source, &w.SearchTerm, &w.Hash, &w.Title, &w.Description, &w.IsFavorite, &w.Status, &w.Brightness, &w.CreatedAt)
		if err != nil {
			continue
		}

		tagRows, err := db.conn.Query(`SELECT t.name FROM tags t JOIN wallpaper_tags wt ON t.id = wt.tag_id WHERE wt.wallpaper_id = ?`, w.ID)
		if err == nil {
			for tagRows.Next() {
				var tag string
				tagRows.Scan(&tag)
				w.Tags = append(w.Tags, tag)
			}
			tagRows.Close()
		}

		wallpapers = append(wallpapers, w)
	}
	return wallpapers, nil
}
