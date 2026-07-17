package scraper

import (
	"encoding/json"
	"fmt"
	"strings"
	"wallpaper-chooser/internal/database"
	"github.com/PuerkitoBio/goquery"
)

var searchTerms = []string{
	"mountains", "ocean", "forest", "city", "sunset",
	"space", "nature", "minimal", "abstract", "dark",
	"cars", "animals", "flowers", "aerial", "desert",
	"night", "beach", "winter", "rain", "clouds",
}

type WallhavenProvider struct {
	engine *Engine
}

func NewWallhaven(engine *Engine) *WallhavenProvider {
	return &WallhavenProvider{engine: engine}
}

func (p *WallhavenProvider) Name() string { return "wallhaven" }

func (p *WallhavenProvider) Scrape(page int) ([]database.Wallpaper, int, error) {
	return p.ScrapeSearch("wallpapers", page)
}

func (p *WallhavenProvider) ScrapeSearch(term string, page int) ([]database.Wallpaper, int, error) {
	apiURL := fmt.Sprintf("https://wallhaven.cc/api/v1/search?q=%s&page=%d&sorting=relevance&order=desc&categories=111&purity=100", term, page)
	resp, err := p.engine.FetchJSON(apiURL)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch wallhaven %s: %w", term, err)
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID         string `json:"id"`
			Path       string `json:"path"`
			Thumbs     struct {
				Small string `json:"small"`
				Med   string `json:"medium"`
				Large string `json:"large"`
			} `json:"thumbs"`
			Resolution string `json:"resolution"`
			FileSize   int64  `json:"file_size"`
			Source     string `json:"source"`
			Tags       []struct {
				Name string `json:"name"`
			} `json:"tags"`
		} `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("parse wallhaven: %w", err)
	}

	var wallpapers []database.Wallpaper
	for _, item := range result.Data {
		w, h := parseResolution(item.Resolution)
		if w == 0 || h == 0 {
			continue
		}
		if item.Path == "" {
			continue
		}

		thumbURL := item.Thumbs.Large
		if thumbURL == "" {
			thumbURL = item.Thumbs.Med
		}
		if thumbURL == "" {
			thumbURL = item.Thumbs.Small
		}

		var tags []string
		for _, t := range item.Tags {
			if t.Name != "" {
				tags = append(tags, t.Name)
			}
		}
		tags = append(tags, term)

		wallpapers = append(wallpapers, database.Wallpaper{
			URL:          item.Path,
			ThumbnailURL: thumbURL,
			Width:        w,
			Height:       h,
			Filesize:     item.FileSize,
			Source:       "wallhaven",
			SearchTerm:   term,
			Title:        fmt.Sprintf("Wallhaven %s", item.ID),
			Tags:         tags,
			Status:       "scraped",
		})
	}

	logf("wallhaven: found %d wallpapers for '%s' page %d", len(wallpapers), term, page)
	return wallpapers, result.Meta.Total, nil
}

type UnsplashProvider struct {
	engine *Engine
}

func NewUnsplash(engine *Engine) *UnsplashProvider {
	return &UnsplashProvider{engine: engine}
}

func (p *UnsplashProvider) Name() string { return "unsplash" }

func (p *UnsplashProvider) Scrape(page int) ([]database.Wallpaper, int, error) {
	return p.ScrapeSearch("wallpapers", page)
}

func (p *UnsplashProvider) ScrapeSearch(term string, page int) ([]database.Wallpaper, int, error) {
	apiURL := fmt.Sprintf("https://unsplash.com/napi/photos?query=%s&per_page=30&page=%d&order_by=relevant", term, page)
	resp, err := p.engine.FetchJSON(apiURL)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch unsplash %s: %w", term, err)
	}
	defer resp.Body.Close()

	var results []struct {
		ID          string `json:"id"`
		Alt         string `json:"alt_description"`
		Description string `json:"description"`
		URLs        struct {
			Full    string `json:"full"`
			Regular string `json:"regular"`
			Small   string `json:"small"`
			Thumb   string `json:"thumb"`
		} `json:"urls"`
		Width  int `json:"width"`
		Height int `json:"height"`
		Tags   []struct {
			Type string `json:"type"`
			Title string `json:"title"`
		} `json:"tags"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, 0, fmt.Errorf("parse unsplash: %w", err)
	}

	var wallpapers []database.Wallpaper
	for _, item := range results {
		dlURL := item.URLs.Regular
		if dlURL == "" {
			dlURL = item.URLs.Full
		}
		if dlURL == "" {
			continue
		}

		thumbURL := item.URLs.Small
		if thumbURL == "" {
			thumbURL = item.URLs.Thumb
		}

		title := item.Alt
		if title == "" {
			title = item.Description
		}

		var tags []string
		for _, t := range item.Tags {
			if t.Title != "" {
				tags = append(tags, t.Title)
			}
		}
		tags = append(tags, term)

		wallpapers = append(wallpapers, database.Wallpaper{
			URL:          dlURL,
			ThumbnailURL: thumbURL,
			Width:        item.Width,
			Height:       item.Height,
			Source:       "unsplash",
			SearchTerm:   term,
			Title:        title,
			Tags:         tags,
			Status:       "scraped",
		})
	}

	logf("unsplash: found %d wallpapers for '%s' page %d", len(wallpapers), term, page)
	return wallpapers, len(results), nil
}

type PexelsProvider struct {
	engine *Engine
}

func NewPexels(engine *Engine) *PexelsProvider {
	return &PexelsProvider{engine: engine}
}

func (p *PexelsProvider) Name() string { return "pexels" }

func (p *PexelsProvider) Scrape(page int) ([]database.Wallpaper, int, error) {
	return p.ScrapeSearch("wallpapers", page)
}

func (p *PexelsProvider) ScrapeSearch(term string, page int) ([]database.Wallpaper, int, error) {
	url := fmt.Sprintf("https://www.pexels.com/search/%s/?page=%d", strings.ReplaceAll(term, " ", "-"), page)
	doc, err := p.engine.FetchDocument(url)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch pexels %s: %w", term, err)
	}

	var wallpapers []database.Wallpaper
	doc.Find("img[data-big-src], img[srcset], img[src]").Each(func(i int, s *goquery.Selection) {
		bigSrc := s.AttrOr("data-big-src", "")
		rawSrc := s.AttrOr("src", "")
		srcset := s.AttrOr("srcset", "")

		var fullURL, thumbURL string

		parsedSrcset := parseSrcset(srcset)

		if bigSrc != "" {
			fullURL = bigSrc
		} else if len(parsedSrcset) > 0 {
			fullURL = parsedSrcset[len(parsedSrcset)-1].url
		} else if rawSrc != "" {
			fullURL = rawSrc
		}

		if len(parsedSrcset) > 0 {
			for _, entry := range parsedSrcset {
				if entry.width > 0 && entry.width <= 400 {
					thumbURL = entry.url
					break
				}
			}
			if thumbURL == "" {
				thumbURL = parsedSrcset[0].url
			}
		} else if rawSrc != "" && strings.HasPrefix(rawSrc, "http") && !strings.Contains(rawSrc, "data:") {
			thumbURL = rawSrc
		}

		if thumbURL == "" {
			thumbURL = fullURL
		}

		if fullURL == "" || !strings.HasPrefix(fullURL, "http") {
			return
		}
		if strings.Contains(fullURL, "avatar") || strings.Contains(fullURL, "logo") || strings.Contains(fullURL, "icon") {
			return
		}

		alt := s.AttrOr("alt", "")

		wallpapers = append(wallpapers, database.Wallpaper{
			URL:          fullURL,
			ThumbnailURL: thumbURL,
			Source:       "pexels",
			SearchTerm:   term,
			Title:        alt,
			Tags:         []string{term},
			Status:       "scraped",
		})
	})

	logf("pexels: found %d wallpapers for '%s' page %d", len(wallpapers), term, page)
	return wallpapers, len(wallpapers), nil
}

type srcsetEntry struct {
	url   string
	width int
}

func parseSrcset(srcset string) []srcsetEntry {
	if srcset == "" {
		return nil
	}
	var entries []srcsetEntry
	parts := strings.Split(srcset, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		entry := srcsetEntry{url: fields[0]}
		if len(fields) > 1 {
			w := strings.TrimSuffix(fields[1], "w")
			fmt.Sscanf(w, "%d", &entry.width)
		}
		entries = append(entries, entry)
	}
	return entries
}
