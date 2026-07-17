package tags

import (
	"strings"

	"wallpaper-chooser/internal/image/colors"
)

// Input carries the analyzed features used to generate semantic tags.
type Input struct {
	DominantColors []string
	Brightness      float64
	Category        string
	Title           string
	SearchTerm      string
	Existing        []string
}

// Generate derives a human-readable, de-duplicated tag list from the visual
// analysis (color names, dark/light, category, plus original tags/keywords).
func Generate(in Input) []string {
	seen := map[string]bool{}
	var out []string
	add := func(t string) {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			return
		}
		seen[t] = true
		out = append(out, t)
	}

	for _, hex := range in.DominantColors {
		if name := colors.ColorName(hex); name != "" {
			add(name)
		}
	}
	if in.Brightness < 0.25 {
		add("dark")
	} else if in.Brightness > 0.75 {
		add("light")
	}
	if in.Category != "" {
		add(in.Category)
	}
	for _, t := range in.Existing {
		add(t)
	}
	for _, t := range strings.Fields(strings.ToLower(in.Title + " " + in.SearchTerm)) {
		t = strings.Trim(t, ".,/:;-")
		if len(t) > 2 {
			add(t)
		}
	}
	return out
}
