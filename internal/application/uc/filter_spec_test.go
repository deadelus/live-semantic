package uc

import "testing"

func TestParseFilterSpec(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []filterTerm
		wantErr bool
	}{
		{"empty string means no filter", "", nil, false},
		{"whitespace-only means no filter", "   ", nil, false},
		{"single exact term, implicit cap 1", "person", []filterTerm{{Key: "person", Cap: 1}}, false},
		{"single exact term, explicit cap", "person*2", []filterTerm{{Key: "person", Cap: 2}}, false},
		{"two independent exact terms", "person*2,car", []filterTerm{{Key: "person", Cap: 2}, {Key: "car", Cap: 1}}, false},
		{"case/whitespace tolerant", "  Person , CAR*3 ", []filterTerm{{Key: "person", Cap: 1}, {Key: "car", Cap: 3}}, false},
		{"non-COCO term is valid — semantic, not rejected", "person with a red hat", []filterTerm{{Key: "person with a red hat", Cap: 1}}, false},
		{"non-COCO term with explicit cap", "person with a red hat*1", []filterTerm{{Key: "person with a red hat", Cap: 1}}, false},
		{"mixed exact and semantic terms", "person*2,person with a red hat*1", []filterTerm{{Key: "person", Cap: 2}, {Key: "person with a red hat", Cap: 1}}, false},
		{"duplicate term rejected (implicit vs implicit)", "person,person", nil, true},
		{"duplicate term rejected (implicit vs explicit)", "person,person*2", nil, true},
		{"duplicate semantic term rejected", "unicorn,unicorn*2", nil, true},
		{"non-numeric cap rejected", "person*abc", nil, true},
		{"zero cap rejected", "person*0", nil, true},
		{"negative cap rejected", "person*-1", nil, true},
		{"stray comma (empty term) rejected", "person,,car", nil, true},
		{"empty term before * rejected", "*2", nil, true},
		{"bare +overlap means true", "person*1+overlap", []filterTerm{{Key: "person", Cap: 1, Overlap: true}}, false},
		{"+overlap=true explicit", "person*1+overlap=true", []filterTerm{{Key: "person", Cap: 1, Overlap: true}}, false},
		{"+overlap=false explicit", "person*1+overlap=false", []filterTerm{{Key: "person", Cap: 1, Overlap: false}}, false},
		{"no +overlap means false by default", "person*1", []filterTerm{{Key: "person", Cap: 1, Overlap: false}}, false},
		{"+overlap on a semantic term, no explicit cap", "person with a red hat+overlap", []filterTerm{{Key: "person with a red hat", Cap: 1, Overlap: true}}, false},
		{"+overlap combined across multiple terms", "person*2+overlap,car", []filterTerm{{Key: "person", Cap: 2, Overlap: true}, {Key: "car", Cap: 1, Overlap: false}}, false},
		{"unknown option rejected", "person*1+bogus", nil, true},
		{"non-bool value for overlap rejected", "person*1+overlap=maybe", nil, true},
		{"repeated option in the same term rejected", "person*1+overlap+overlap", nil, true},
		{"empty option (stray +) rejected", "person*1+", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFilterSpec(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseFilterSpec(%q) error = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseFilterSpec(%q) = %+v, want %+v", tt.raw, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("parseFilterSpec(%q)[%d] = %+v, want %+v", tt.raw, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIsCOCOLabel(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"person", true},
		{"car", true},
		{"banana", true}, // yes, a real COCO class — verified against entities.Yolo11sClasses()
		{"unicorn", false},
		{"person with a red hat", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := isCOCOLabel(tt.key); got != tt.want {
				t.Fatalf("isCOCOLabel(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestSemanticLabelHint(t *testing.T) {
	tests := []struct {
		key       string
		wantLabel string
		wantOK    bool
	}{
		{"person with a yellow hat", "person", true},
		{"a red car", "car", true},
		{"person near a car", "", false},     // two COCO classes mentioned — ambiguous, no hint
		{"a unicorn", "", false},             // no COCO class mentioned at all
		{"scary carpet salesman", "", false}, // "car" must not match inside "scary"/"carpet" — word-boundary check
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			gotLabel, gotOK := semanticLabelHint(tt.key)
			if gotOK != tt.wantOK || gotLabel != tt.wantLabel {
				t.Fatalf("semanticLabelHint(%q) = (%q, %v), want (%q, %v)", tt.key, gotLabel, gotOK, tt.wantLabel, tt.wantOK)
			}
		})
	}
}

func TestContainsWord(t *testing.T) {
	tests := []struct {
		haystack, needle string
		want             bool
	}{
		{"person with a yellow hat", "person", true},
		{"a hat and a person", "person", true},
		{"scary carpet", "car", false},
		{"a car nearby", "car", true},
		{"car", "car", true},
		{"cars", "car", false}, // "cars" isn't the word "car" — trailing 's' is a word byte
	}

	for _, tt := range tests {
		t.Run(tt.haystack+"/"+tt.needle, func(t *testing.T) {
			if got := containsWord(tt.haystack, tt.needle); got != tt.want {
				t.Fatalf("containsWord(%q, %q) = %v, want %v", tt.haystack, tt.needle, got, tt.want)
			}
		})
	}
}
