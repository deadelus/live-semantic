package gocvtracker

import "testing"

func TestAlgorithmString(t *testing.T) {
	tests := []struct {
		name string
		algo Algorithm
		want string
	}{
		{"KCF", KCF, "KCF"},
		{"CSRT", CSRT, "CSRT"},
		{"unknown", Algorithm(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.algo.String(); got != tt.want {
				t.Errorf("Algorithm(%d).String() = %q, want %q", tt.algo, got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		algo    Algorithm
		wantErr bool
	}{
		{"KCF is supported", KCF, false},
		{"CSRT is supported", CSRT, false},
		{"unsupported algorithm errors", Algorithm(99), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trk, err := New(tt.algo)
			if (err != nil) != tt.wantErr {
				t.Fatalf("New(%v) error = %v, wantErr %v", tt.algo, err, tt.wantErr)
			}
			if !tt.wantErr && trk == nil {
				t.Fatal("New() returned nil Tracker without error")
			}
		})
	}
}

func TestCleanupWithoutInitIsSafe(t *testing.T) {
	trk, err := New(KCF)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	// Must not panic even though Init was never called.
	trk.Cleanup()
	trk.Cleanup()
}
