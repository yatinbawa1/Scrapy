package search

import (
	"wallpaper-chooser/internal/database"
)

type Engine struct {
	db *database.DB
}

func New(db *database.DB) *Engine {
	return &Engine{db: db}
}

func (e *Engine) Search(query, source string, minW, minH, page, pageSize int) (database.SearchResult, error) {
	if pageSize <= 0 {
		pageSize = 50
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize
	return e.db.Search(query, source, minW, minH, pageSize, offset)
}

func (e *Engine) Browse(page, pageSize int, favorites bool) ([]database.Wallpaper, int, error) {
	return e.BrowseSorted(page, pageSize, favorites, "latest")
}

func (e *Engine) BrowseSorted(page, pageSize int, favorites bool, sortBy string) ([]database.Wallpaper, int, error) {
	return e.BrowseSortedFiltered(page, pageSize, favorites, sortBy, "", "", "")
}

func (e *Engine) BrowseSortedFiltered(page, pageSize int, favorites bool, sortBy string, searchTerm string, query string, source string) ([]database.Wallpaper, int, error) {
	if pageSize <= 0 {
		pageSize = 50
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize

	if favorites {
		w, err := e.db.GetFavorites(pageSize, offset)
		return w, len(w), err
	}

	return e.db.GetAllSortedFiltered(pageSize, offset, sortBy, searchTerm, query, source)
}
