package database

import (
	"database/sql"
	"fmt"
	"os"
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
	}

	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			return fmt.Errorf("exec %q: %w", q[:40], err)
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
