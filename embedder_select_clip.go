//go:build clip

package main

import (
	"log"

	"wallpaper-chooser/internal/ai/clip"
	"wallpaper-chooser/internal/ai/embeddings"
)

// newEmbedder prefers a real CLIP model when its files are available,
// otherwise falls back to the heuristic embedder so the app still runs.
func newEmbedder(modelDir string) embeddings.Embedder {
	// Best-effort: fetch the model + runtime if missing. A failure here just
	// means we fall back to the heuristic embedder below.
	if err := clip.EnsureModels(modelDir); err != nil {
		log.Printf("[clip] model setup skipped (%v); using heuristic embedder", err)
	}
	if e, err := clip.New(modelDir); err == nil {
		return e
	}
	return embeddings.HeuristicEmbedder{}
}
