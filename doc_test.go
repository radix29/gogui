package gogui

import (
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// parseDoc returns the documentation of the package in dir, without its
// test files, as go doc sees it (exported identifiers only).
func parseDoc(t *testing.T, dir, path string) *doc.Package {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Every non-test file, whatever its build tags (driver_darwin.go and
	// driver_other.go both count).
	var files []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	d, err := doc.NewFromFiles(fset, files, path)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// mentions reports whether text names id as a whole word.
func mentions(text, id string) bool {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(id) + `\b`).MatchString(text)
}

// TestEveryExportDocumented fails on an exported identifier of gogui
// without a doc comment (S6.3c): a function, a type, a method (the ones
// promoted from the embedded generic types included), a constant or a
// variable (the comment of its group, its own one or its line comment).
// An exported field of an exported struct needs its own comment or a
// mention in its type's doc. A type aliased from internal/driver must name
// every exported field and method of its target in its doc, because go doc
// shows neither for an alias.
func TestEveryExportDocumented(t *testing.T) {
	p := parseDoc(t, ".", "github.com/radix29/gogui")
	drv := parseDoc(t, "internal/driver", "github.com/radix29/gogui/internal/driver")
	if strings.TrimSpace(p.Doc) == "" {
		t.Error("package gogui has no package doc")
	}
	values := func(kind string, vs []*doc.Value) {
		for _, v := range vs {
			for _, sp := range v.Decl.Specs {
				s := sp.(*ast.ValueSpec)
				for _, n := range s.Names {
					if n.IsExported() && v.Doc == "" && s.Doc == nil && s.Comment == nil {
						t.Errorf("%s %s has no doc comment", kind, n.Name)
					}
				}
			}
		}
	}
	funcs := func(prefix string, fs []*doc.Func) {
		for _, f := range fs {
			if strings.TrimSpace(f.Doc) == "" {
				t.Errorf("func %s%s has no doc comment", prefix, f.Name)
			}
		}
	}
	driverTypes := map[string]*doc.Type{}
	for _, dt := range drv.Types {
		driverTypes[dt.Name] = dt
	}
	values("const", p.Consts)
	values("var", p.Vars)
	funcs("", p.Funcs)
	for _, typ := range p.Types {
		if strings.TrimSpace(typ.Doc) == "" {
			t.Errorf("type %s has no doc comment", typ.Name)
		}
		values("const", typ.Consts)
		values("var", typ.Vars)
		funcs("", typ.Funcs)
		funcs(typ.Name+".", typ.Methods)
		spec := typ.Decl.Specs[0].(*ast.TypeSpec)
		if st, ok := spec.Type.(*ast.StructType); ok {
			for _, f := range st.Fields.List {
				for _, n := range f.Names {
					if n.IsExported() && f.Doc == nil && f.Comment == nil && !mentions(typ.Doc, n.Name) {
						t.Errorf("field %s.%s has no comment and the type's doc doesn't name it", typ.Name, n.Name)
					}
				}
			}
		}
		if !spec.Assign.IsValid() {
			continue
		}
		sel, ok := spec.Type.(*ast.SelectorExpr)
		if ok && sel.X.(*ast.Ident).Name == "mainthread" && sel.Sel.Name == "PanicError" {
			continue // DoSync's panic value (A7); its fields are documented there
		}
		if !ok || sel.X.(*ast.Ident).Name != "driver" {
			t.Errorf("alias %s: target is not a driver type", typ.Name)
			continue
		}
		target := driverTypes[sel.Sel.Name]
		if target == nil {
			t.Errorf("alias %s: driver.%s not found", typ.Name, sel.Sel.Name)
			continue
		}
		var names []string
		if st, ok := target.Decl.Specs[0].(*ast.TypeSpec).Type.(*ast.StructType); ok {
			for _, f := range st.Fields.List {
				for _, n := range f.Names {
					if n.IsExported() {
						names = append(names, n.Name)
					}
				}
			}
		}
		for _, m := range target.Methods {
			names = append(names, m.Name)
		}
		for _, n := range names {
			if !mentions(typ.Doc, n) {
				t.Errorf("alias %s: its doc doesn't name driver.%s's %s, which go doc doesn't show", typ.Name, sel.Sel.Name, n)
			}
		}
	}
}
