package repocheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"iter"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The example tests of the root package (examples_test.go) build the trees
// of examples/*/main.go a second time, because a package main can't be
// imported and an internal gogui test can't import a package that imports
// gogui. TestExampleTreesMatch keeps the two copies from drifting apart
// (review R14): it reads the public-API calls that build each tree from
// both sources and fails when they differ.
//
// A tree is the multiset of its construction calls: every call of an
// exported gogui function (NewButton, VStack, Row, Spacer, BoldSystemFont,
// …) with the methods chained on it (.SetDefault(true), .SetPadding(20)),
// and every WindowConfig literal. Literal arguments are kept, so a changed
// title, spacing or range is drift; a variable, a closure or a nested
// construction call reads as "_", and a nested call is a tree element of
// its own, so binding a row to a variable first changes nothing.

// treeBuilders names, per example, the functions of examples_test.go that
// build its tree.
var treeBuilders = map[string][]string{
	"hello":     {"exampleHello"},
	"form":      {"newFormExample"},
	"converter": {"exampleConverter"},
	"controls":  {"newControlsExample"},
	"gallery":   {"exampleGallery"},
	"notes":     {"exampleNotes"},
}

// untracked are exported gogui functions that build no tree.
var untracked = map[string]bool{"Run": true, "Do": true, "DoSync": true}

func TestExampleTreesMatch(t *testing.T) {
	fset := token.NewFileSet()
	testFile, err := parser.ParseFile(fset, "examples_test.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	funcs := map[string]*ast.FuncDecl{}
	for _, d := range testFile.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil {
			funcs[fd.Name.Name] = fd
		}
	}
	for _, d := range slices.Sorted(keysOf(examplesList(t))) {
		names, ok := treeBuilders[d]
		if !ok {
			t.Errorf("examples/%s: no entry in treeBuilders", d)
			continue
		}
		mainFile, err := parser.ParseFile(fset, filepath.Join("examples", d, "main.go"), nil, 0)
		if err != nil {
			t.Error(err)
			continue
		}
		app := tree(mainFile, "gogui")
		var test []string
		for _, n := range names {
			fd := funcs[n]
			if fd == nil {
				t.Errorf("examples_test.go: no func %s (the %s tree)", n, d)
				continue
			}
			test = append(test, tree(fd.Body, "")...)
		}
		if len(app) == 0 {
			t.Errorf("examples/%s/main.go: no tree found", d)
			continue
		}
		onlyApp, onlyTest := diff(app, test)
		onlyApp, onlyTest = matchWild(onlyApp, onlyTest)
		for _, s := range onlyApp {
			t.Errorf("%s: examples/%s/main.go builds %s, examples_test.go (%s) does not",
				d, d, s, strings.Join(names, ", "))
		}
		for _, s := range onlyTest {
			t.Errorf("%s: examples_test.go (%s) builds %s, examples/%s/main.go does not",
				d, strings.Join(names, ", "), s, d)
		}
	}
}

// TestTreeSignatures checks the reader on a small source: chains, nested
// calls, literals and variables.
func TestTreeSignatures(t *testing.T) {
	src := `package main
func main() {
	gogui.Run(gogui.AppConfig{Name: "X"}, func(app *gogui.App) {
		b := gogui.NewButton("OK").SetDefault(true)
		win := app.NewWindow(gogui.WindowConfig{Title: "T", Size: gogui.Size{W: 3, H: 4}, Style: gogui.Titled | gogui.Closable})
		win.SetContent(gogui.VStack(8, gogui.HStack(4, gogui.Spacer(), b), gogui.NewSlider().SetRange(-40, 100)).SetPadding(20))
		gogui.Do(func() { b.SetTitle("x") })
	})
}`
	f, err := parser.ParseFile(token.NewFileSet(), "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := tree(f, "gogui")
	want := []string{
		`HStack(4, _, _)`,
		`NewButton("OK").SetDefault(true)`,
		`NewSlider().SetRange(-40, 100)`,
		`Spacer()`,
		`VStack(8, _, _).SetPadding(20)`,
		`WindowConfig{Size: Size{H: 4, W: 3}, Style: Titled | Closable, Title: "T"}`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("tree:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// tree returns the sorted construction calls under n. pkg is the qualifier
// of the gogui package in the source ("" inside package gogui).
func tree(n ast.Node, pkg string) []string {
	r := reader{pkg: pkg}
	ast.Inspect(n, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if r.chainHead(x) && !r.isChainLink(x) {
				r.out = append(r.out, r.call(x))
			}
		case *ast.CompositeLit:
			if r.goguiName(x.Type) == "WindowConfig" {
				r.out = append(r.out, r.expr(x))
			}
		}
		return true
	})
	sort.Strings(r.out)
	return r.out
}

type reader struct {
	pkg   string
	out   []string
	links map[*ast.CallExpr]bool // calls that are the receiver of a chained method
}

// goguiName returns the name e refers to in package gogui ("" if none).
func (r *reader) goguiName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		if r.pkg == "" && ast.IsExported(x.Name) {
			return x.Name
		}
	case *ast.SelectorExpr:
		if id, ok := x.X.(*ast.Ident); ok && r.pkg != "" && id.Name == r.pkg {
			return x.Sel.Name
		}
	}
	return ""
}

// tracked reports whether c calls an exported gogui function that builds
// part of a tree.
func (r *reader) tracked(c *ast.CallExpr) bool {
	n := r.goguiName(c.Fun)
	return n != "" && !untracked[n]
}

// chainHead reports whether c is a tracked call or a method chained on one,
// and marks the receivers of the chain as links.
func (r *reader) chainHead(c *ast.CallExpr) bool {
	if r.tracked(c) {
		return true
	}
	sel, ok := c.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	inner, ok := sel.X.(*ast.CallExpr)
	if !ok || !r.chainHead(inner) {
		return false
	}
	if r.links == nil {
		r.links = map[*ast.CallExpr]bool{}
	}
	r.links[inner] = true
	return true
}

func (r *reader) isChainLink(c *ast.CallExpr) bool { return r.links[c] }

// call renders a tracked call with its chain.
func (r *reader) call(c *ast.CallExpr) string {
	var head string
	if sel, ok := c.Fun.(*ast.SelectorExpr); ok && r.goguiName(c.Fun) == "" {
		head = r.call(sel.X.(*ast.CallExpr)) + "." + sel.Sel.Name
	} else {
		head = r.goguiName(c.Fun)
	}
	args := make([]string, len(c.Args))
	for i, a := range c.Args {
		args[i] = r.expr(a)
	}
	return head + "(" + strings.Join(args, ", ") + ")"
}

// expr renders an argument: literals and gogui constants as written,
// anything else as "_".
func (r *reader) expr(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.BasicLit:
		return x.Value
	case *ast.Ident:
		switch {
		case x.Name == "true" || x.Name == "false" || x.Name == "nil":
			return x.Name
		case r.goguiName(x) != "":
			return x.Name
		}
	case *ast.SelectorExpr:
		if n := r.goguiName(x); n != "" {
			return n
		}
	case *ast.ParenExpr:
		return "(" + r.expr(x.X) + ")"
	case *ast.UnaryExpr:
		return x.Op.String() + r.expr(x.X)
	case *ast.BinaryExpr:
		return r.expr(x.X) + " " + x.Op.String() + " " + r.expr(x.Y)
	case *ast.CompositeLit:
		var elts []string
		for _, el := range x.Elts {
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				elts = append(elts, key(kv.Key)+": "+r.expr(kv.Value))
			} else {
				elts = append(elts, r.expr(el))
			}
		}
		sort.Strings(elts)
		return r.goguiName(x.Type) + "{" + strings.Join(elts, ", ") + "}"
	}
	return "_"
}

// matchWild pairs each element of app with "_" in it (a value main.go
// computes, such as a title from its argument) with an element of test
// that has any value there, and returns the elements left unpaired.
func matchWild(app, test []string) (restApp, restTest []string) {
	used := make([]bool, len(test))
	for _, a := range app {
		re := wildRe(a)
		paired := false
		for i, b := range test {
			if !used[i] && re != nil && re.MatchString(b) {
				used[i], paired = true, true
				break
			}
		}
		if !paired {
			restApp = append(restApp, a)
		}
	}
	for i, b := range test {
		if !used[i] {
			restTest = append(restTest, b)
		}
	}
	return restApp, restTest
}

// wildRe returns a regexp that matches s with each "_" outside a string
// literal standing for any value, or nil when s has none.
func wildRe(s string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	wild, inStr := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inStr && c == '\\' && i+1 < len(s):
			b.WriteString(regexp.QuoteMeta(s[i : i+2]))
			i++
			continue
		case c == '"':
			inStr = !inStr
		case !inStr && c == '_':
			b.WriteString(".+?")
			wild = true
			continue
		}
		b.WriteString(regexp.QuoteMeta(string(c)))
	}
	if !wild {
		return nil
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// diff returns the elements of a missing from b and of b missing from a,
// counting duplicates.
func diff(a, b []string) (onlyA, onlyB []string) {
	n := map[string]int{}
	for _, s := range a {
		n[s]++
	}
	for _, s := range b {
		n[s]--
	}
	for _, s := range slices.Sorted(keysOf(n)) {
		for ; n[s] > 0; n[s]-- {
			onlyA = append(onlyA, s)
		}
		for ; n[s] < 0; n[s]++ {
			onlyB = append(onlyB, s)
		}
	}
	return onlyA, onlyB
}

// key renders the key of a composite literal element: a field name as
// written.
func key(e ast.Expr) string {
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return "_"
}

// keysOf returns the keys of m.
func keysOf[V any](m map[string]V) iter.Seq[string] {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}
