package runtime

import "testing"

func TestIsAtLeast(t *testing.T) {
	cases := []struct {
		got, min string
		want     bool
	}{
		{"1.22.0", "1.20.0", true},
		{"1.20.0", "1.20.0", true},
		{"1.19.9", "1.20.0", false},
		{"2.0.0", "1.99.99", true},
		{"1.20.1", "1.20.0", true},
		{"1.20.0", "1.20.1", false},
	}
	for _, c := range cases {
		got, err := isAtLeast(c.got, c.min)
		if err != nil {
			t.Fatalf("isAtLeast(%q, %q) unexpected error: %v", c.got, c.min, err)
		}
		if got != c.want {
			t.Errorf("isAtLeast(%q, %q) = %v, want %v", c.got, c.min, got, c.want)
		}
	}
}

func TestParseVersionInvalid(t *testing.T) {
	if _, err := parseVersion("1.20"); err == nil {
		t.Error("expected error for missing patch segment")
	}
	if _, err := parseVersion("1.x.0"); err == nil {
		t.Error("expected error for non-numeric segment")
	}
}
