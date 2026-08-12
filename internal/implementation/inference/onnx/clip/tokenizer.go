// This file implements CLIP's tokenizer: byte-level BPE (identical
// algorithm to GPT-2's, plus a "</w>" end-of-word marker), from scratch —
// no third-party Go dependency. Rationale in docs/adr/clip-backend.md § 4:
// no established/actively-maintained Go library specifically compatible
// with CLIP's tokenizer was found, and this is too correctness-critical
// (it directly decides what a natural-language filter matches) to bet on
// one of uncertain maintenance. The algorithm itself has been stable and
// publicly documented since 2021 (openai/CLIP, clip/simple_tokenizer.py) —
// a bounded, well-defined port, not an open-ended risk.
package clip

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

//go:embed data/vocab.json data/merges.txt
var tokenizerData embed.FS

const (
	startOfText = "<|startoftext|>"
	endOfText   = "<|endoftext|>"

	// contextLength is CLIP's fixed text sequence length — the text
	// encoder expects exactly this many token IDs per input, padded with
	// endOfText or truncated.
	contextLength = 77
)

// bpePair is a merge rule: two adjacent symbols that combine into one,
// ranked by the order they appear in merges.txt (lower rank = applied
// first — merges.txt is itself ordered by frequency during CLIP's original
// training, most common merges first).
type bpePair struct {
	first, second string
}

// tokenizer implements CLIP's byte-level BPE tokenizer.
type tokenizer struct {
	encoder     map[string]int32
	bpeRanks    map[bpePair]int
	byteEncoder [256]rune
	pattern     *regexp.Regexp
	cache       map[string]string
}

// newTokenizer loads the embedded vocab.json/merges.txt (see data/,
// sourced from openai/clip-vit-base-patch32 — docs/adr/clip-backend.md)
// and compiles the pre-tokenization pattern.
func newTokenizer() (*tokenizer, error) {
	vocabBytes, err := tokenizerData.ReadFile("data/vocab.json")
	if err != nil {
		return nil, fmt.Errorf("clip: read vocab.json: %w", err)
	}
	var encoder map[string]int32
	if err := json.Unmarshal(vocabBytes, &encoder); err != nil {
		return nil, fmt.Errorf("clip: parse vocab.json: %w", err)
	}

	mergesFile, err := tokenizerData.Open("data/merges.txt")
	if err != nil {
		return nil, fmt.Errorf("clip: open merges.txt: %w", err)
	}
	defer mergesFile.Close()

	bpeRanks := make(map[bpePair]int, 48894)
	scanner := bufio.NewScanner(mergesFile)
	lineNum := 0
	rank := 0
	for scanner.Scan() {
		lineNum++
		if lineNum == 1 {
			continue // "#version: 0.2 ..." header line, not a merge rule
		}
		line := scanner.Text()
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		bpeRanks[bpePair{parts[0], parts[1]}] = rank
		rank++
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("clip: read merges.txt: %w", err)
	}

	// (?i): CLIP's reference pattern is compiled case-insensitive (the
	// contraction alternatives 's/'t/'re/... need to match "'S"/"'T" too);
	// harmless no-op for the \p{L}/\p{N} classes. Go's RE2 regexp supports
	// \p{L}/\p{N} natively, no third-party regex engine needed here
	// (unlike the Python reference, which needs the third-party `regex`
	// module for this — stdlib `re` doesn't support \p{L}/\p{N}).
	pattern, err := regexp.Compile(`(?i)<\|startoftext\|>|<\|endoftext\|>|'s|'t|'re|'ve|'m|'ll|'d|[\p{L}]+|[\p{N}]|[^\s\p{L}\p{N}]+`)
	if err != nil {
		return nil, fmt.Errorf("clip: compile pretokenize pattern: %w", err)
	}

	return &tokenizer{
		encoder:     encoder,
		bpeRanks:    bpeRanks,
		byteEncoder: byteToUnicode(),
		pattern:     pattern,
		cache:       make(map[string]string),
	}, nil
}

// byteToUnicode maps each possible byte value (0-255) to a printable rune,
// so that arbitrary UTF-8 bytes (including control characters) can be
// represented as regular characters before BPE — the same trick GPT-2's
// tokenizer uses to run byte-level BPE on any input without ever needing
// an "unknown byte" token. Printable ASCII/Latin-1 bytes map to
// themselves; every other byte maps to a sequential codepoint starting at
// 256 (byte 0x20, space, is the best-known example — it lands on U+0120,
// often rendered as "Ġ" in GPT-2/CLIP vocab dumps).
func byteToUnicode() [256]rune {
	inRange := func(b int) bool {
		return (b >= '!' && b <= '~') || (b >= 0xA1 && b <= 0xAC) || (b >= 0xAE && b <= 0xFF)
	}

	var result [256]rune
	n := 0
	for b := 0; b < 256; b++ {
		if inRange(b) {
			result[b] = rune(b)
		} else {
			result[b] = rune(256 + n)
			n++
		}
	}
	return result
}

// getPairs returns every adjacent (word[i], word[i+1]) pair in word.
func getPairs(word []string) []bpePair {
	if len(word) < 2 {
		return nil
	}
	pairs := make([]bpePair, 0, len(word)-1)
	for i := 0; i < len(word)-1; i++ {
		pairs = append(pairs, bpePair{word[i], word[i+1]})
	}
	return pairs
}

// lowestRankPair returns the pair among pairs with the lowest merge rank
// (i.e. the merge rule to apply first), and whether any of pairs is an
// actual known merge rule at all.
func lowestRankPair(pairs []bpePair, ranks map[bpePair]int) (bpePair, bool) {
	best := bpePair{}
	bestRank := 0
	found := false
	for _, p := range pairs {
		r, ok := ranks[p]
		if !ok {
			continue
		}
		if !found || r < bestRank {
			found = true
			bestRank = r
			best = p
		}
	}
	return best, found
}

// bpe applies byte-pair merges to a single already byte-encoded token
// (i.e. every rune already went through byteEncoder), returning the
// space-separated final subword pieces — direct port of CLIP's reference
// bpe() (clip/simple_tokenizer.py). token is not the raw input text: it's
// one pre-tokenized chunk (see Encode), already run through byteEncoder.
func (t *tokenizer) bpe(token string) string {
	if cached, ok := t.cache[token]; ok {
		return cached
	}

	runes := []rune(token)
	if len(runes) == 0 {
		return token
	}

	word := make([]string, len(runes))
	for i, r := range runes {
		word[i] = string(r)
	}
	word[len(word)-1] += "</w>"

	// Single-symbol token: no pair to merge, matches the reference's
	// early-return when get_pairs() finds nothing to do.
	if len(word) == 1 {
		t.cache[token] = word[0]
		return word[0]
	}

	for {
		pairs := getPairs(word)
		bigram, found := lowestRankPair(pairs, t.bpeRanks)
		if !found {
			break
		}

		newWord := make([]string, 0, len(word))
		i := 0
		for i < len(word) {
			j := indexOfFrom(word, bigram.first, i)
			if j == -1 {
				newWord = append(newWord, word[i:]...)
				break
			}
			newWord = append(newWord, word[i:j]...)
			i = j

			if word[i] == bigram.first && i < len(word)-1 && word[i+1] == bigram.second {
				newWord = append(newWord, bigram.first+bigram.second)
				i += 2
			} else {
				newWord = append(newWord, word[i])
				i++
			}
		}
		word = newWord

		if len(word) == 1 {
			break
		}
	}

	result := strings.Join(word, " ")
	t.cache[token] = result
	return result
}

// indexOfFrom returns the index of the first occurrence of s in word at
// or after from, or -1 if not found.
func indexOfFrom(word []string, s string, from int) int {
	for i := from; i < len(word); i++ {
		if word[i] == s {
			return i
		}
	}
	return -1
}

// whitespaceClean collapses any run of whitespace to a single space and
// trims the ends — matches CLIP's reference whitespace_clean().
func whitespaceClean(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// encode tokenizes text into raw BPE token IDs — no <|startoftext|>/
// <|endoftext|> wrapping or padding, see Encode for that.
func (t *tokenizer) encode(text string) []int32 {
	text = whitespaceClean(strings.ToLower(text))

	var ids []int32
	for _, chunk := range t.pattern.FindAllString(text, -1) {
		var encoded strings.Builder
		for _, b := range []byte(chunk) {
			encoded.WriteRune(t.byteEncoder[b])
		}
		for _, piece := range strings.Split(t.bpe(encoded.String()), " ") {
			if id, ok := t.encoder[piece]; ok {
				ids = append(ids, id)
			}
			// A missing piece would mean a byte-encoded character that
			// isn't in the vocab at all — shouldn't happen, the vocab's
			// base 256 entries cover every possible byteEncoder output
			// individually, so BPE always bottoms out at worst to known
			// single-character pieces. Silently dropping rather than
			// erroring: this mirrors the reference implementation, which
			// has no explicit handling for this either (relies on the
			// same vocab-completeness guarantee).
		}
	}
	return ids
}

// Encode tokenizes text into CLIP's fixed-length input format: wrapped
// with <|startoftext|>/<|endoftext|>, then padded with <|endoftext|> up to
// contextLength (77) or truncated (keeping room for the trailing
// <|endoftext|>) if longer.
func (t *tokenizer) Encode(text string) []int32 {
	ids := t.encode(text)

	maxContent := contextLength - 2 // room for start + end tokens
	if len(ids) > maxContent {
		ids = ids[:maxContent]
	}

	result := make([]int32, 0, contextLength)
	result = append(result, t.encoder[startOfText])
	result = append(result, ids...)
	result = append(result, t.encoder[endOfText])
	for len(result) < contextLength {
		result = append(result, t.encoder[endOfText])
	}
	return result
}
