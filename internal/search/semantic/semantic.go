// Package semantic ranks wallpapers by natural-language query using CLIP-style
// embeddings (provided by an embeddings.Embedder) combined with a light keyword
// boost.
package semantic

import (
	"strings"

	"wallpaper-chooser/internal/ai/embeddings"
	"wallpaper-chooser/internal/search"
)

// RankText embeds the query and ranks items by cosine similarity to stored
// embeddings, with a small boost when query terms appear in the item's text.
func RankText(embedder embeddings.Embedder, items []search.Item, query string, limit int) ([]search.Result, error) {
	qv, err := embedder.EmbedText(query)
	if err != nil {
		return nil, err
	}

	qTokens := tokenize(query)
	var results []search.Result
	for _, it := range items {
		if len(it.Embedding) == 0 {
			continue
		}
		score := embeddings.Cosine(qv, it.Embedding)
		if len(qTokens) > 0 {
			hits := 0
			lower := strings.ToLower(it.Text)
			for _, t := range qTokens {
				if strings.Contains(lower, t) {
					hits++
				}
			}
			score += 0.2 * float64(hits) / float64(len(qTokens))
		}
		results = append(results, search.Result{ID: it.ID, Score: score})
	}

	sortResults(results)
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func tokenize(s string) []string {
	parts := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
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

func sortResults(r []search.Result) {
	for i := 1; i < len(r); i++ {
		for j := i; j > 0 && r[j].Score > r[j-1].Score; j-- {
			r[j], r[j-1] = r[j-1], r[j]
		}
	}
}
