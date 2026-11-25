package repocheck

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestExamplesListed checks that every folder of examples/ is registered
// where the examples are listed (examples/README.md, convention 7): the
// `examples` list of the root package's aqua_test.go with the app's name as
// its main.go sets it, a row in the README's table and an entry under its
// "Keys and menus", and that nothing is listed without a folder.
// scripts/native.sh derives its list from the folders.
func TestExamplesListed(t *testing.T) {
	entries, err := os.ReadDir("examples")
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	if len(dirs) == 0 {
		t.Fatal("no example folders")
	}
	readme, err := os.ReadFile(filepath.Join("examples", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	listed := examplesList(t)
	for _, d := range dirs {
		app, ok := listed[d]
		if !ok {
			t.Errorf("examples/%s is not in the examples list of aqua_test.go", d)
		} else if src, err := os.ReadFile(filepath.Join("examples", d, "main.go")); err != nil {
			t.Error(err)
		} else if !strings.Contains(string(src), fmt.Sprintf("AppConfig{Name: %q", app)) {
			t.Errorf("examples/%s/main.go does not set AppConfig.Name %q", d, app)
		}
		if row := fmt.Sprintf("| [`%s/`](%s) |", d, d); !strings.Contains(string(readme), row) {
			t.Errorf("examples/README.md has no table row %s", row)
		}
		if entry := fmt.Sprintf("* **`%s`**:", d); !strings.Contains(string(readme), entry) {
			t.Errorf("examples/README.md has no \"Keys and menus\" entry %s", entry)
		}
		delete(listed, d)
	}
	for name := range listed {
		t.Errorf("aqua_test.go lists %s, which has no folder in examples/", name)
	}
}

// examplesList returns the folder → app name entries of the `examples`
// list in aqua_test.go, read from its source ({"hello", "Hello", …} rows).
func examplesList(t *testing.T) map[string]string {
	t.Helper()
	src, err := os.ReadFile("aqua_test.go")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)\nvar examples = \[\]struct \{.*?\n\}\{\n(.*?)\n\}\n`).FindSubmatch(src)
	if block == nil {
		t.Fatal("aqua_test.go: no `var examples = []struct{…}{…}` list")
	}
	row := regexp.MustCompile(`(?m)^\t\{"([^"]+)", "([^"]+)", \w+\},$`)
	out := map[string]string{}
	for _, line := range strings.Split(string(block[1]), "\n") {
		m := row.FindStringSubmatch(line)
		if m == nil {
			t.Fatalf("aqua_test.go: examples row %q is not {\"dir\", \"App\", builder},", line)
		}
		out[m[1]] = m[2]
	}
	return out
}
