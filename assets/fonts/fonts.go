// Package fonts embeds the font files of assets/fonts (PLAN.md S2.5g): the
// Inter TTFs the user adds there, or none. The pattern also matches the
// directory's README.md, so the package builds without any font.
package fonts

import (
	"embed"
	"path"
	"slices"
	"strings"
)

//go:embed *
var dir embed.FS

// File is one embedded font file.
type File struct {
	Name string // the file name in assets/fonts
	Data []byte // shared with the binary's data; do not modify
}

// Files returns the embedded .ttf and .otf files (any case), sorted by name.
// It is empty when the directory holds no font.
func Files() []File {
	entries, err := dir.ReadDir(".")
	if err != nil { // the root of an embed.FS always exists
		panic("fonts: " + err.Error())
	}
	var out []File
	for _, e := range entries {
		if e.IsDir() || !IsFontFile(e.Name()) {
			continue
		}
		data, err := dir.ReadFile(e.Name())
		if err != nil {
			panic("fonts: " + err.Error())
		}
		out = append(out, File{Name: e.Name(), Data: data})
	}
	slices.SortFunc(out, func(a, b File) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// IsFontFile reports whether name has a font file extension (.ttf, .otf).
func IsFontFile(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".ttf", ".otf":
		return true
	}
	return false
}
