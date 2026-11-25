package repocheck

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The documents name files and packages as anchors; they are not complete
// file lists (each package directory is). These tests keep the anchors
// real (review D4): every Go file a document names exists, every package
// is named in ARCHITECTURE.md, and the OS-specific files lie only where
// ARCHITECTURE.md §1 says build tags are.

// docs are the documents whose file names are checked. docs/review-plan.md
// (which names what is wrong) and docs/spikes.md (which cites the Go
// runtime's files) are not.
var docs = []string{
	"ARCHITECTURE.md", "PLAN.md", "CLAUDE.md", "README.md", "THIRD_PARTY.md",
	"examples/README.md", "docs/parity.md",
}

// repoFiles returns the slash paths of the files under the repository
// root, without .git and the uncommitted logs.
func repoFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(".", func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "native-logs", "screenshots", ".claude":
				return filepath.SkipDir
			}
			return nil
		}
		out = append(out, filepath.ToSlash(p))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDocsNameExistingFiles(t *testing.T) {
	files := repoFiles(t)
	paths, bases := map[string]bool{}, map[string]bool{}
	for _, f := range files {
		paths[f] = true
		bases[path.Base(f)] = true
	}
	ref := regexp.MustCompile(`[A-Za-z0-9_./*-]+\.go\b`)
	for _, doc := range docs {
		b, err := os.ReadFile(doc)
		if err != nil {
			t.Error(err)
			continue
		}
		for _, name := range ref.FindAllString(string(b), -1) {
			if strings.Contains(name, "*") {
				continue // a pattern such as *_linux.go
			}
			if paths[name] || paths[path.Join(path.Dir(doc), name)] || paths["internal/"+name] {
				continue
			}
			if !bases[path.Base(name)] {
				t.Errorf("%s names %s, which does not exist", doc, name)
			}
		}
	}
}

func TestPackagesNamed(t *testing.T) {
	arch, err := os.ReadFile("ARCHITECTURE.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(arch)
	for _, dir := range goDirs(t) {
		if dir == "." || strings.HasPrefix(dir, "examples/") || strings.HasPrefix(dir, "spike/") {
			continue // gogui itself; the examples (TestExamplesListed) and spikes are listed by folder
		}
		short := strings.TrimPrefix(dir, "internal/")
		if strings.Contains(doc, "`"+dir) || strings.Contains(doc, "`"+short) ||
			strings.Contains(doc, "`"+path.Base(dir)+"`") {
			continue
		}
		t.Errorf("ARCHITECTURE.md does not name the package %s", dir)
	}
}

// goDirs returns the directories that hold non-test Go files.
func goDirs(t *testing.T) []string {
	t.Helper()
	seen := map[string]bool{}
	var out []string
	for _, f := range repoFiles(t) {
		if strings.HasSuffix(f, ".go") && !strings.HasSuffix(f, "_test.go") && !strings.Contains(f, "/testdata/") {
			if d := path.Dir(f); !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
	}
	return out
}

// osFile matches a file name with a GOOS or GOARCH suffix.
var osFile = regexp.MustCompile(`_(windows|linux|darwin|freebsd|netbsd|openbsd|unix|amd64|arm64|386|arm)(_test)?\.go$`)

// TestBuildTagsWhereDocumented checks ARCHITECTURE.md §1's rule: OS
// constraints (a //go:build line or a GOOS/GOARCH file name) only in
// gogui's driver-selection files, internal/ffi, internal/shell/native, the
// Windows packages under internal/shell/win32, the *_linux.go files of
// internal/shell/gtk4 (and gtk4/gtk), and spike/. Tests follow the same
// rule as the code they test.
func TestBuildTagsWhereDocumented(t *testing.T) {
	for _, f := range repoFiles(t) {
		if !strings.HasSuffix(f, ".go") || strings.Contains(f, "/testdata/") {
			continue
		}
		constrained := osFile.MatchString(f) || hasBuildLine(t, f)
		if !constrained {
			continue
		}
		switch {
		case f == "driver_darwin.go" || f == "driver_other.go",
			strings.HasPrefix(f, "internal/ffi/"),
			strings.HasPrefix(f, "internal/shell/native/"),
			strings.HasPrefix(f, "internal/shell/win32/"),
			strings.HasPrefix(f, "internal/shell/gtk4/") && (strings.HasSuffix(f, "_linux.go") || strings.HasSuffix(f, "_linux_test.go")),
			strings.HasPrefix(f, "spike/"):
		default:
			t.Errorf("%s has an OS constraint outside the places ARCHITECTURE.md §1 lists", f)
		}
	}
}

// hasBuildLine reports whether f has a //go:build line before its package
// clause.
func hasBuildLine(t *testing.T, f string) bool {
	t.Helper()
	fh, err := os.Open(f)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "//go:build ") {
			return true
		}
		if strings.HasPrefix(line, "package ") {
			return false
		}
	}
	return false
}
