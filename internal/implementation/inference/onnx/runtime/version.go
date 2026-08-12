package runtime

import (
	"fmt"
	"strconv"
	"strings"

	ort "github.com/yalue/onnxruntime_go"
)

// RequireMinVersion fails fast if the linked ONNX Runtime shared library is
// older than min (e.g. "1.20.0"). Must be called after InitEnvironment.
// Prevents silently running against a library that predates an opset or
// Execution Provider feature the model/session config relies on.
func RequireMinVersion(min string) error {
	got := ort.GetVersion()
	ok, err := isAtLeast(got, min)
	if err != nil {
		return fmt.Errorf("parsing onnxruntime version: %w", err)
	}
	if !ok {
		return fmt.Errorf("onnxruntime %s does not satisfy minimum required version %s", got, min)
	}
	return nil
}

// isAtLeast compares two "major.minor.patch"-style version strings.
func isAtLeast(got, min string) (bool, error) {
	g, err := parseVersion(got)
	if err != nil {
		return false, err
	}
	m, err := parseVersion(min)
	if err != nil {
		return false, err
	}
	for i := 0; i < 3; i++ {
		if g[i] != m[i] {
			return g[i] > m[i], nil
		}
	}
	return true, nil
}

// parseVersion splits a "major.minor.patch" string into its three
// integer components, erroring on any non-numeric or malformed segment.
func parseVersion(v string) ([3]int, error) {
	var out [3]int
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return out, fmt.Errorf("expected major.minor.patch, got %q", v)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return out, fmt.Errorf("invalid version segment %q in %q: %w", p, v, err)
		}
		out[i] = n
	}
	return out, nil
}
