package main

import (
	"path/filepath"
	"testing"

	"wallpaper-chooser/internal/ai/embeddings"
	"wallpaper-chooser/internal/database"
)

func TestSemanticSearchCustomLabel(t *testing.T) {
	dir := t.TempDir()
	db, err := database.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	defer db.Close()

	e := embeddings.HeuristicEmbedder{}

	insert := func(id int64, title, searchTerm string, custom []string, emb []float32) {
		w := &database.Wallpaper{
			ID:         id,
			URL:        "http://example.com/" + string(rune('0'+id)),
			Source:     "test",
			SearchTerm: searchTerm,
			Title:      title,
			Status:     "downloaded",
		}
		if _, err := db.InsertWallpaper(w); err != nil {
			t.Fatalf("insert wallpaper %d: %v", id, err)
		}
		m := &database.Metadata{WallpaperID: id, Embedding: emb, CustomLabels: custom, Brightness: 0.5}
		if err := db.UpsertMetadata(m); err != nil {
			t.Fatalf("upsert metadata %d: %v", id, err)
		}
	}

	// Wallpaper tagged "arch" with NO text metadata at all.
	archEmb, _ := e.EmbedAnalysis(embeddings.AnalysisInput{CustomLabels: []string{"arch"}})
	insert(1, "", "", []string{"arch"}, archEmb)

	// Unrelated control wallpaper.
	forestEmb, _ := e.EmbedAnalysis(embeddings.AnalysisInput{CustomLabels: []string{"forest"}})
	insert(2, "", "", []string{"forest"}, forestEmb)

	// A wallpaper whose title literally says "Arch Linux".
	textEmb, _ := e.EmbedAnalysis(embeddings.AnalysisInput{Title: "Arch Linux", SearchTerm: "archlinux"})
	insert(3, "Arch Linux", "archlinux", nil, textEmb)

	a := &App{db: db, embedder: e}

	res := a.SemanticSearch("arch", 2)
	if len(res) == 0 {
		t.Fatal("expected results for query 'arch'")
	}

	gotIDs := make([]int64, len(res))
	for i, w := range res {
		gotIDs[i] = w.ID
	}
	t.Logf("query 'arch' -> ids %v", gotIDs)

	// The tagged wallpaper (id 1) and the "Arch Linux" wallpaper (id 3) must both
	// be present; the forest wallpaper (id 2) must not.
	has := func(id int64) bool {
		for _, x := range gotIDs {
			if x == id {
				return true
			}
		}
		return false
	}
	if !has(1) {
		t.Errorf("custom-tagged wallpaper #1 should be found via tag 'arch'")
	}
	if !has(3) {
		t.Errorf("'Arch Linux' wallpaper #3 should be found")
	}
	if has(2) {
		t.Errorf("unrelated 'forest' wallpaper #2 should NOT be returned for 'arch'")
	}
}
