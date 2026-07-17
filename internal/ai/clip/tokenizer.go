//go:build clip

package clip

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

// Tokenizer implements CLIP's byte-level BPE tokenizer using vocab.json and
// merges.txt, matching the OpenAI CLIP text encoder exactly.
type Tokenizer struct {
	vocab       map[string]int
	ranks       map[string]int
	byteEncoder map[byte]rune
	special     map[string]int
}

func NewTokenizer(vocabPath, mergesPath string) (*Tokenizer, error) {
	vb, err := os.ReadFile(vocabPath)
	if err != nil {
		return nil, err
	}
	var vocab map[string]int
	if err := json.Unmarshal(vb, &vocab); err != nil {
		return nil, err
	}

	mb, err := os.ReadFile(mergesPath)
	if err != nil {
		return nil, err
	}
	ranks := map[string]int{}
	for i, line := range strings.Split(string(mb), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Some exports prefix merges.txt with a single integer count line.
		if i == 0 {
			if _, err := strconv.Atoi(line); err == nil {
				continue
			}
		}
		if parts := strings.SplitN(line, " ", 2); len(parts) == 2 {
			ranks[line] = i
		}
	}

	return &Tokenizer{
		vocab:       vocab,
		ranks:       ranks,
		byteEncoder: bytesToUnicode(),
		special:     map[string]int{"<|startoftext|>": 49406, "<|endoftext|>": 49407},
	}, nil
}

// Encode turns text into exactly 77 token ids (CLIP's fixed sequence length),
// padded with 0 and framed by the SOT/EOT special tokens.
func (t *Tokenizer) Encode(text string) []int {
	text = basicClean(text)
	text = strings.ToLower(text)

	ids := []int{t.special["<|startoftext|>"]}
	for _, word := range strings.Fields(text) {
		var chars []string
		for _, b := range []byte(word) {
			chars = append(chars, string(t.byteEncoder[b]))
		}
		for _, tok := range bpe(chars, t.ranks) {
			if id, ok := t.vocab[tok]; ok {
				ids = append(ids, id)
			}
		}
	}
	ids = append(ids, t.special["<|endoftext|>"])

	if len(ids) > 77 {
		ids = ids[:77]
	}
	for len(ids) < 77 {
		ids = append(ids, 0)
	}
	return ids
}

func bpe(word []string, ranks map[string]int) []string {
	if len(word) <= 1 {
		return word
	}
	for {
		bestRank := int(^uint(0) >> 1)
		bestIdx := -1
		for i := 0; i < len(word)-1; i++ {
			pair := word[i] + " " + word[i+1]
			if r, ok := ranks[pair]; ok && r < bestRank {
				bestRank = r
				bestIdx = i
			}
		}
		if bestIdx == -1 {
			break
		}
		word[bestIdx] = word[bestIdx] + word[bestIdx+1]
		word = append(word[:bestIdx+1], word[bestIdx+2:]...)
		if len(word) == 1 {
			break
		}
	}
	return word
}

// bytesToUnicode mirrors OpenAI CLIP's reversible byte->unicode mapping so the
// BPE operates on a safe character set.
func bytesToUnicode() map[byte]rune {
	bs := []byte{}
	for b := 33; b <= 126; b++ {
		bs = append(bs, byte(b))
	}
	for b := 161; b <= 172; b++ {
		bs = append(bs, byte(b))
	}
	for b := 174; b <= 255; b++ {
		bs = append(bs, byte(b))
	}
	cs := make([]rune, len(bs))
	for i, b := range bs {
		cs[i] = rune(b)
	}
	n := 0
	for b := 0; b < 256; b++ {
		found := false
		for _, x := range bs {
			if x == byte(b) {
				found = true
				break
			}
		}
		if !found {
			bs = append(bs, byte(b))
			cs = append(cs, rune(256+n))
			n++
		}
	}
	m := map[byte]rune{}
	for i, b := range bs {
		m[b] = cs[i]
	}
	return m
}

func basicClean(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}
