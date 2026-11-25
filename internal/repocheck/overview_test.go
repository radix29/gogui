package repocheck

import (
	"os"
	"strings"
	"testing"
)

// TestOverviewMatchesPlan checks that ARCHITECTURE.md §0 is the copy of
// PLAN.md §2 and §2.1 it says it is (review D5; before, a manual rule of
// CLAUDE.md): the same diagram and the same layer table, up to the
// "PLAN.md " that ARCHITECTURE.md puts before a PLAN section reference.
func TestOverviewMatchesPlan(t *testing.T) {
	plan := section(t, "PLAN.md", "## 2. Architecture Overview", "## 3.")
	arch := section(t, "ARCHITECTURE.md", "## 0. Target architecture overview", "### Implemented today")
	if p, a := fenced(t, "PLAN.md §2", plan), fenced(t, "ARCHITECTURE.md §0", arch); p != a {
		t.Errorf("the diagrams differ:\nPLAN.md §2:\n%s\nARCHITECTURE.md §0:\n%s", p, a)
	}
	pt := table(t, "PLAN.md §2.1", plan[strings.Index(plan, "### 2.1 Layers"):])
	at := table(t, "ARCHITECTURE.md §0", arch)
	at = strings.ReplaceAll(at, "PLAN.md §", "§")
	if pt != at {
		t.Errorf("the layer tables differ:\nPLAN.md §2.1:\n%s\nARCHITECTURE.md §0:\n%s", pt, at)
	}
}

// section returns the text of file from the line start to the next line
// that begins with end.
func section(t *testing.T, file, start, end string) string {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	i := strings.Index(s, "\n"+start+"\n")
	if i < 0 {
		t.Fatalf("%s: no heading %q", file, start)
	}
	s = s[i+1:]
	if j := strings.Index(s, "\n"+end); j >= 0 {
		s = s[:j]
	}
	return s
}

// fenced returns the first ``` block of s.
func fenced(t *testing.T, where, s string) string {
	t.Helper()
	i := strings.Index(s, "\n```\n")
	if i < 0 {
		t.Fatalf("%s: no ``` block", where)
	}
	s = s[i+5:]
	j := strings.Index(s, "\n```\n")
	if j < 0 {
		t.Fatalf("%s: unterminated ``` block", where)
	}
	return s[:j]
}

// table returns the first Markdown table of s (its consecutive "|" lines).
func table(t *testing.T, where, s string) string {
	t.Helper()
	var rows []string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "|") {
			rows = append(rows, line)
		} else if len(rows) > 0 {
			break
		}
	}
	if len(rows) == 0 {
		t.Fatalf("%s: no table", where)
	}
	return strings.Join(rows, "\n")
}
