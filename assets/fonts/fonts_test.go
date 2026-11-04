package fonts

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestIsFontFile(t *testing.T) {
	for name, want := range map[string]bool{
		"Inter-Regular.ttf": true, "Inter-Bold.TTF": true, "x.otf": true, "y.OtF": true,
		"README.md": false, "OFL.txt": false, "fonts.go": false, "ttf": false, "a.ttf.txt": false, "": false,
	} {
		if got := IsFontFile(name); got != want {
			t.Errorf("IsFontFile(%q) = %v, want %v", name, got, want)
		}
	}
}

// TestFiles: only font files, sorted, with their data; the README is
// embedded (so the pattern always matches) but not returned.
func TestFiles(t *testing.T) {
	files := Files()
	if !slices.IsSortedFunc(files, func(a, b File) int { return strings.Compare(a.Name, b.Name) }) {
		t.Error("files not sorted")
	}
	for _, f := range files {
		if !IsFontFile(f.Name) || len(f.Data) == 0 {
			t.Errorf("file %q (%d bytes)", f.Name, len(f.Data))
		}
	}
	readme, err := dir.ReadFile("README.md")
	if err != nil || !bytes.Contains(readme, []byte("Inter")) {
		t.Errorf("README.md not embedded: %v", err)
	}
}
