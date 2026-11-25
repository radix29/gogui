package repocheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestParityChecklist keeps docs/parity.md (S6.3f) consistent: every check
// a table cell names is defined in its §4, every open check is named by at
// least one cell, each check is defined once, every cell of the target
// matrices starts with a state mark, and the README, the examples README
// and native.sh point to the file.
func TestParityChecklist(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("docs", "parity.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(src)
	defRe := regexp.MustCompile(`(?m)^- \*\*([MWLAGC][0-9]+)\*\*`)
	refRe := regexp.MustCompile(`\b([MWLAGC][0-9]+)\b`)

	defined := map[string]bool{}
	for _, m := range defRe.FindAllStringSubmatch(doc, -1) {
		if defined[m[1]] {
			t.Errorf("check %s is defined twice", m[1])
		}
		defined[m[1]] = true
	}
	if len(defined) == 0 {
		t.Fatal("no checks defined")
	}

	referenced := map[string]bool{}
	section := ""
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(line, "## ") {
			section = line
		}
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for _, c := range cells[1:] {
			for _, m := range refRe.FindAllStringSubmatch(c, -1) {
				referenced[m[1]] = true
				if !defined[m[1]] {
					t.Errorf("table row %q names %s, which §4 does not define", strings.TrimSpace(cells[0]), m[1])
				}
			}
		}
		// The target matrices of §3 (six columns): every state cell
		// starts with a mark.
		if strings.HasPrefix(section, "## 3.") && len(cells) == 6 && !strings.HasPrefix(strings.TrimSpace(cells[1]), "---") &&
			strings.TrimSpace(cells[1]) != "macOS native" {
			for _, c := range cells[1:] {
				c = strings.TrimSpace(c)
				if !strings.HasPrefix(c, "✔") && !strings.HasPrefix(c, "◐") &&
					!strings.HasPrefix(c, "○") && !strings.HasPrefix(c, "—") {
					t.Errorf("row %q has a cell without a state mark: %q", strings.TrimSpace(cells[0]), c)
				}
			}
		}
	}
	for id := range defined {
		if !strings.HasPrefix(id, "C") && !referenced[id] {
			t.Errorf("open check %s is named by no table cell", id)
		}
	}

	for _, f := range []string{"README.md", filepath.Join("examples", "README.md"), filepath.Join("scripts", "native.sh")} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Error(err)
			continue
		}
		if !strings.Contains(string(b), "docs/parity.md") {
			t.Errorf("%s does not point to docs/parity.md", f)
		}
	}
}
