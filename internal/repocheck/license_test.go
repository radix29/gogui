package repocheck

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// TestThirdPartyNotices keeps THIRD_PARTY.md in step with go.mod (S6.3b): every module
// go.mod requires, direct or indirect, has a row in its modules table with the same
// version, and every row names a required module. A new dependency therefore fails here
// until its license is recorded. It also checks that LICENSE is the MIT License (D12) and
// that the bundled font's license files are present.
func TestThirdPartyNotices(t *testing.T) {
	required := goModRequires(t, "go.mod")
	if len(required) == 0 {
		t.Fatal("go.mod: no require found")
	}
	listed := thirdPartyModules(t, "THIRD_PARTY.md")
	for mod, ver := range required {
		got, ok := listed[mod]
		switch {
		case !ok:
			t.Errorf("THIRD_PARTY.md: no row for %s %s (go.mod requires it; add its license)", mod, ver)
		case got != ver:
			t.Errorf("THIRD_PARTY.md: %s is listed at %s, go.mod requires %s", mod, got, ver)
		}
	}
	for mod := range listed {
		if _, ok := required[mod]; !ok {
			t.Errorf("THIRD_PARTY.md: row for %s, which go.mod does not require", mod)
		}
	}

	lic, err := os.ReadFile("LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(lic), "MIT License\n") {
		t.Errorf("LICENSE: does not start with %q", "MIT License")
	}
	for _, f := range []string{"assets/fonts/OFL.txt", "assets/fonts/LICENSE.txt"} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("bundled font license: %v", err)
		}
	}
}

// goModRequires returns the module paths and versions of every require directive in
// the go.mod at path, in both the block and the single-line form.
func goModRequires(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	mods := map[string]string{}
	inBlock := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		switch {
		case len(fields) == 0:
		case inBlock && fields[0] == ")":
			inBlock = false
		case inBlock && len(fields) >= 2:
			mods[fields[0]] = fields[1]
		case fields[0] == "require" && len(fields) == 2 && fields[1] == "(":
			inBlock = true
		case fields[0] == "require" && len(fields) >= 3:
			mods[fields[1]] = fields[2]
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return mods
}

// thirdPartyModules returns the module paths and versions of the table under the
// "## Go modules" heading of THIRD_PARTY.md: rows whose first cell is a backquoted path.
func thirdPartyModules(t *testing.T, path string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	mods := map[string]string{}
	inSection := false
	for _, line := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "## ") {
			inSection = line == "## Go modules"
			continue
		}
		if !inSection || !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) < 3 {
			continue
		}
		mod := strings.TrimSpace(cells[0])
		if len(mod) < 3 || mod[0] != '`' || mod[len(mod)-1] != '`' {
			continue
		}
		mods[strings.Trim(mod, "`")] = strings.TrimSpace(cells[1])
	}
	return mods
}
