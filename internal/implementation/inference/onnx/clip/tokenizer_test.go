package clip

import (
	"testing"
)

// Reference vectors generated independently in Python, porting CLIP's
// original algorithm (openai/CLIP, clip/simple_tokenizer.py) against the
// same vocab.json/merges.txt embedded here — see docs/adr/clip-backend.md
// § 4. Not copied from a library: computed directly against these exact
// data files to catch any porting mistake, including the Unicode-letter
// classification edge case ("café" must stay one token, not "caf"+"é" —
// caught and fixed during that reference generation itself).
func TestTokenizerEncodeContentIDs(t *testing.T) {
	tok, err := newTokenizer()
	if err != nil {
		t.Fatalf("newTokenizer() error = %v", err)
	}

	tests := []struct {
		name string
		text string
		want []int32
	}{
		{"simple filter", "a red backpack", []int32{320, 736, 14894}},
		{"multi-word", "a photo of a dog", []int32{320, 1125, 539, 320, 1929}},
		{"punctuation", "hello, world!", []int32{3306, 267, 1002, 256}},
		{"unicode letter stays whole", "café", []int32{15304}},
		{"contraction + hyphen", "it's a test-case", []int32{585, 568, 320, 1628, 268, 2068}},
		{"digits", "42 apples", []int32{275, 273, 14032}},
		{"case insensitive", "MiXeD Case", []int32{6780, 2068}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tok.encode(tt.text)
			if !equalInt32(got, tt.want) {
				t.Errorf("encode(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestTokenizerEncodeWrapsAndPads(t *testing.T) {
	tok, err := newTokenizer()
	if err != nil {
		t.Fatalf("newTokenizer() error = %v", err)
	}

	got := tok.Encode("a red backpack")
	if len(got) != contextLength {
		t.Fatalf("len(Encode(...)) = %d, want %d (contextLength)", len(got), contextLength)
	}

	startID := tok.encoder[startOfText]
	endID := tok.encoder[endOfText]
	want := []int32{startID, 320, 736, 14894, endID}
	for i, id := range want {
		if got[i] != id {
			t.Errorf("Encode(...)[%d] = %d, want %d", i, got[i], id)
		}
	}
	// Everything after the content + first endOfText should be padding
	// (endOfText repeated) up to contextLength.
	for i := len(want); i < contextLength; i++ {
		if got[i] != endID {
			t.Errorf("Encode(...)[%d] = %d, want %d (padding)", i, got[i], endID)
		}
	}
}

func TestTokenizerEncodeTruncatesLongInput(t *testing.T) {
	tok, err := newTokenizer()
	if err != nil {
		t.Fatalf("newTokenizer() error = %v", err)
	}

	// Way more than contextLength-2 content tokens once tokenized.
	long := ""
	for i := 0; i < 200; i++ {
		long += "word "
	}

	got := tok.Encode(long)
	if len(got) != contextLength {
		t.Fatalf("len(Encode(long)) = %d, want %d", len(got), contextLength)
	}
	if got[0] != tok.encoder[startOfText] {
		t.Errorf("Encode(long)[0] = %d, want startOfText id", got[0])
	}
	if got[contextLength-1] != tok.encoder[endOfText] {
		t.Errorf("Encode(long)[last] = %d, want endOfText id", got[contextLength-1])
	}
}

func TestByteToUnicodeIsBijective(t *testing.T) {
	mapping := byteToUnicode()
	seen := make(map[rune]bool, 256)
	for b, r := range mapping {
		if seen[r] {
			t.Fatalf("byteToUnicode(): rune %U (from byte %d) collides with a previous byte's mapping", r, b)
		}
		seen[r] = true
	}
	// Spot-check: space (0x20) is the classic case that lands outside the
	// printable ranges and gets remapped — must not map to itself.
	if mapping[' '] == ' ' {
		t.Error("byteToUnicode()[' '] should be remapped, not identity")
	}
	// Printable ASCII '!' must map to itself (first byte in the "keep as
	// is" range).
	if mapping['!'] != '!' {
		t.Errorf("byteToUnicode()['!'] = %c, want '!' (identity)", mapping['!'])
	}
}

func equalInt32(a, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
