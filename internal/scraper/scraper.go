package scraper

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"wallpaper-chooser/internal/database"
	"github.com/PuerkitoBio/goquery"
)

type Provider interface {
	Name() string
	Scrape(page int) ([]database.Wallpaper, int, error)
	ScrapeSearch(term string, page int) ([]database.Wallpaper, int, error)
}

type Engine struct {
	providers map[string]Provider
	client    *http.Client
}

func NewEngine() *Engine {
	return &Engine{
		providers: make(map[string]Provider),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (e *Engine) Register(p Provider) {
	e.providers[p.Name()] = p
}

func (e *Engine) Providers() []string {
	var names []string
	for n := range e.providers {
		names = append(names, n)
	}
	return names
}

func (e *Engine) Scrape(source string, page int) ([]database.Wallpaper, int, error) {
	p, ok := e.providers[source]
	if !ok {
		return nil, 0, fmt.Errorf("unknown source: %s", source)
	}
	return p.Scrape(page)
}

func (e *Engine) ScrapeSearch(source, term string, page int) ([]database.Wallpaper, int, error) {
	p, ok := e.providers[source]
	if !ok {
		return nil, 0, fmt.Errorf("unknown source: %s", source)
	}
	return p.ScrapeSearch(term, page)
}

func (e *Engine) FetchDocument(rawURL string) (*goquery.Document, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status code %d for %s", resp.StatusCode, rawURL)
	}

	return goquery.NewDocumentFromReader(resp.Body)
}

func (e *Engine) FetchJSON(rawURL string) (*http.Response, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")
	return e.client.Do(req)
}

func parseResolution(s string) (int, int) {
	parts := strings.Split(s, "x")
	if len(parts) == 2 {
		w, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
		h, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
		return w, h
	}
	return 0, 0
}

func logf(format string, args ...interface{}) {
	log.Printf("[scraper] "+format, args...)
}
