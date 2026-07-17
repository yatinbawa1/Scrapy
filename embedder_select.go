//go:build !clip

package main

import "wallpaper-chooser/internal/ai/embeddings"

// newEmbedder returns the default heuristic embedder. When the app is built
// with the "clip" build tag, embedder_select_clip.go overrides this to use a
// real CLIP model when its files are present, falling back to the heuristic.
func newEmbedder(modelDir string) embeddings.Embedder {
	return embeddings.HeuristicEmbedder{}
}
