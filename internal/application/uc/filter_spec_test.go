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
		{"single label, implicit cap 1", "person", []filterTerm{{Label: "person", Cap: 1}}, false},
		{"single label, explicit cap", "person*2", []filterTerm{{Label: "person", Cap: 2}}, false},
		{"two independent terms", "person*2,car", []filterTerm{{Label: "person", Cap: 2}, {Label: "car", Cap: 1}}, false},
		{"case/whitespace tolerant", "  Person , CAR*3 ", []filterTerm{{Label: "person", Cap: 1}, {Label: "car", Cap: 3}}, false},
		{"unknown label rejected", "unicorn", nil, true},
		{"unknown label rejected even with a cap", "unicorn*2", nil, true},
		{"duplicate label rejected (implicit vs implicit)", "person,person", nil, true},
		{"duplicate label rejected (implicit vs explicit)", "person,person*2", nil, true},
		{"non-numeric cap rejected", "person*abc", nil, true},
		{"zero cap rejected", "person*0", nil, true},
		{"negative cap rejected", "person*-1", nil, true},
		{"stray comma (empty term) rejected", "person,,car", nil, true},
		{"empty label before * rejected", "*2", nil, true},
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
