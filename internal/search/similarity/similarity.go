// Package similarity ranks wallpapers by visual similarity to a target embedding
// (used by "Find Similar").
package similarity

import (
	"wallpaper-chooser/internal/ai/embeddings"
	"wallpaper-chooser/internal/search"
)

// Rank returns items most similar to target (excluding targetID), best first.
func Rank(target []float32, items []search.Item, targetID int64, limit int) []search.Result {
	if len(target) == 0 {
		return nil
	}
	var results []search.Result
	for _, it := range items {
		if it.ID == targetID || len(it.Embedding) == 0 {
			continue
		}
		results = append(results, search.Result{ID: it.ID, Score: embeddings.Cosine(target, it.Embedding)})
	}
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].Score > results[j-1].Score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}
