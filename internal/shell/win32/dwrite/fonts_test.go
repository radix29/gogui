//go:build windows

package dwrite

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/radix29/gogui/internal/shell/win32/com"
)

var (
	vtFontFileLoader         = com.IUnknownVtable.Extend("CreateStreamFromKey")
	vtInMemoryFontFileLoader = vtFontFileLoader.Extend("CreateInMemoryFontFileReference", "GetFileCount")
	vtFontFile               = com.IUnknownVtable.Extend("GetReferenceKey", "GetLoader", "Analyze")
	// AddFontFaceReference's two overloads in MSVC's reverse order.
	vtFontSetBuilder = com.IUnknownVtable.Extend("AddFontFaceReference(properties)",
		"AddFontFaceReference", "AddFontSet", "CreateFontSet")
	vtFontSetBuilder1 = vtFontSetBuilder.Extend("AddFontFile")
	vtFontSet         = com.IUnknownVtable.Extend("GetFontCount", "GetFontFaceReference",
		"FindFontFaceReference", "FindFontFace", "GetPropertyValues(index)",
		"GetPropertyValues(locales)", "GetPropertyValues", "GetPropertyOccurrenceCount",
		"GetMatchingFonts(properties)", "GetMatchingFonts(family)")
)

func TestFontSlots(t *testing.T) {
	if len(vtInMemoryFontFileLoader) != 6 || len(vtFontFile) != 6 || len(vtFontSetBuilder1) != 8 ||
		len(vtFontSet) != 13 {
		t.Errorf("vtable lengths %d, %d, %d, %d; want 6, 6, 8, 13", len(vtInMemoryFontFileLoader),
			len(vtFontFile), len(vtFontSetBuilder1), len(vtFontSet))
	}
	com.CheckSlots(t, "IDWriteFactory5", vtFactory5, map[string]int{
		"RegisterFontFileLoader":          slotFactoryRegisterFontFileLoader,
		"UnregisterFontFileLoader":        slotFactoryUnregisterFontFileLoader,
		"CreateFontCollectionFromFontSet": slotFactoryCreateFontCollectionFromFontSet,
		"CreateFontSetBuilder5":           slotFactoryCreateFontSetBuilder5,
		"CreateInMemoryFontFileLoader":    slotFactoryCreateInMemoryFontFileLoader,
	})
	com.CheckSlots(t, "IDWriteFontCollection", vtFontCollection, map[string]int{
		"FindFamilyName": slotCollectionFindFamilyName,
	})
	com.CheckSlots(t, "IDWriteInMemoryFontFileLoader", vtInMemoryFontFileLoader, map[string]int{
		"CreateInMemoryFontFileReference": slotMemLoaderCreateInMemoryFontFileReference,
		"GetFileCount":                    slotMemLoaderGetFileCount,
	})
	com.CheckSlots(t, "IDWriteFontFile", vtFontFile, map[string]int{"Analyze": slotFontFileAnalyze})
	com.CheckSlots(t, "IDWriteFontSetBuilder1", vtFontSetBuilder1, map[string]int{
		"CreateFontSet": slotBuilderCreateFontSet,
		"AddFontFile":   slotBuilderAddFontFile,
	})
	com.CheckSlots(t, "IDWriteFontSet", vtFontSet, map[string]int{"GetFontCount": slotFontSetGetFontCount})
}

func systemFont(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(os.Getenv("WINDIR"), "Fonts", name))
	if err != nil {
		t.Fatalf("reading the system font %s: %v", name, err)
	}
	return data
}

// width lays text out in family from collection at 13 DIPs on one line and
// returns its width.
func width(t *testing.T, fac Factory, family string, c FontCollection, text string) float32 {
	t.Helper()
	tf, err := fac.CreateTextFormat(family, c, FONT_WEIGHT_NORMAL, FONT_STYLE_NORMAL, FONT_STRETCH_NORMAL, 13, "en-us")
	if err != nil {
		t.Fatal(err)
	}
	defer tf.Release()
	if err := tf.SetWordWrapping(WORD_WRAPPING_NO_WRAP); err != nil {
		t.Fatal(err)
	}
	l, err := fac.CreateTextLayout(u16(text), tf, 1000, 500)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	m, err := l.Metrics()
	if err != nil {
		t.Fatal(err)
	}
	return m.Width
}

// TestInMemoryFonts builds a collection from arial.ttf read into memory,
// step by step through the bindings: the loader copies the bytes (the
// buffer is overwritten right after), the file is a supported TrueType
// face, the set has one font, the collection finds Arial (not Segoe UI),
// text in it is as wide as through the system collection, and every
// reference goes back, the loader unregistered.
func TestInMemoryFonts(t *testing.T) {
	fac, err := CreateFactory()
	if err != nil {
		t.Fatal(err)
	}
	defer fac.Release()
	warmUp(t, fac)
	start := fac.RefCount()

	loader, err := fac.CreateInMemoryFontFileLoader()
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "loader", loader.Ptr, &IID_IDWriteInMemoryFontFileLoader, &IID_IDWriteFontFileLoader)
	if err := fac.RegisterFontFileLoader(loader); err != nil {
		t.Fatal(err)
	}
	if n := loader.RefCount(); n != 2 {
		t.Errorf("registered loader: count %d, want 2 (ours and the factory's)", n)
	}
	if err := fac.RegisterFontFileLoader(loader); err == nil {
		t.Error("second RegisterFontFileLoader: no error")
	}

	data := systemFont(t, "arial.ttf")
	file, err := loader.CreateFontFileReference(fac, data)
	if err != nil {
		t.Fatal(err)
	}
	for i := range data { // the loader holds its own copy
		data[i] = 0
	}
	runtime.GC()
	queries(t, "font file", file.Ptr, &IID_IDWriteFontFile)
	if n := loader.FileCount(); n != 1 {
		t.Errorf("FileCount %d, want 1", n)
	}
	a, err := file.Analyze()
	if err != nil {
		t.Fatal(err)
	}
	// DWRITE_FONT_FILE_TYPE_TRUETYPE is 2, DWRITE_FONT_FACE_TYPE_TRUETYPE 1.
	t.Logf("arial.ttf: %+v", a)
	if !a.Supported || a.FaceCount != 1 || a.FileType != 2 || a.FaceType != 1 {
		t.Errorf("Analyze: %+v, want supported, one TrueType face", a)
	}
	if _, err := loader.CreateFontFileReference(fac, nil); err == nil {
		t.Error("empty data: no error")
	}

	builder, err := fac.CreateFontSetBuilder()
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "builder", builder.Ptr, &IID_IDWriteFontSetBuilder1, &IID_IDWriteFontSetBuilder)
	if err := builder.AddFontFile(file); err != nil {
		t.Fatal(err)
	}
	set, err := builder.CreateFontSet()
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "font set", set.Ptr, &IID_IDWriteFontSet)
	if n := set.FontCount(); n != 1 {
		t.Errorf("FontCount %d, want 1", n)
	}
	coll, err := fac.CreateFontCollectionFromFontSet(set)
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "collection", coll.Ptr, &IID_IDWriteFontCollection, &IID_IDWriteFontCollection1)
	if n := coll.FamilyCount(); n != 1 {
		t.Errorf("FamilyCount %d, want 1", n)
	}
	for _, c := range []struct {
		name   string
		exists bool
	}{{"Arial", true}, {"arial", true}, {"Segoe UI", false}, {"", false}} {
		i, ok, err := coll.FindFamilyName(c.name)
		if err != nil || ok != c.exists || i != 0 {
			t.Errorf("FindFamilyName(%q) = %d, %v, %v; want 0, %v", c.name, i, ok, err, c.exists)
		}
	}
	if _, _, err := coll.FindFamilyName("Ar\x00ial"); err == nil {
		t.Error("FindFamilyName with NUL: no error")
	}

	sys, err := fac.SystemFontCollection(false)
	if err != nil {
		t.Fatal(err)
	}
	if i, ok, err := sys.FindFamilyName("Arial"); err != nil || !ok {
		t.Errorf("system FindFamilyName(Arial) = %d, %v, %v", i, ok, err)
	}
	if _, ok, err := sys.FindFamilyName("No Such Family gogui"); err != nil || ok {
		t.Errorf("system FindFamilyName(unknown) = %v, %v", ok, err)
	}
	const text = "Hello, World 0123"
	wm := width(t, fac, "Arial", coll, text)
	ws := width(t, fac, "Arial", sys, text)
	wseg := width(t, fac, "Segoe UI", sys, text)
	t.Logf("%q at 13 DIPs: Arial from memory %v, from the system %v; Segoe UI %v", text, wm, ws, wseg)
	if wm != ws || wm == wseg {
		t.Errorf("Arial from memory %v, from the system %v, Segoe UI %v", wm, ws, wseg)
	}
	sys.Release()

	release(t, "font file", &file.Ptr, 0) // the set holds no reference to it
	release(t, "builder", &builder.Ptr, 0)
	release(t, "font set", &set.Ptr, 0)
	// The collection works without the file, builder and set.
	if w := width(t, fac, "Arial", coll, text+"Q"); w <= wm {
		t.Errorf("after releasing the set: width %v, want more than %v", w, wm)
	}
	// The loader stays registered until the collection and everything
	// made with it is gone: a layout after UnregisterFontFileLoader kills
	// the process (DirectWrite can't reopen the file's stream).
	release(t, "collection", &coll.Ptr, 0)
	if err := fac.UnregisterFontFileLoader(loader); err != nil {
		t.Error(err)
	}
	if err := fac.UnregisterFontFileLoader(loader); err == nil {
		t.Error("second UnregisterFontFileLoader: no error")
	}
	release(t, "loader", &loader.Ptr, 0)
	if n := fac.RefCount(); n != start {
		t.Errorf("factory count %d, want %d at the start", n, start)
	}
}

// TestMemoryFonts checks the CreateMemoryFonts helper: two system fonts in
// one collection, a format from it, every count back after Release; and
// corrupt, truncated or no data as errors with nothing left behind.
func TestMemoryFonts(t *testing.T) {
	fac, err := CreateFactory()
	if err != nil {
		t.Fatal(err)
	}
	defer fac.Release()
	warmUp(t, fac)
	start := fac.RefCount()

	arial := systemFont(t, "arial.ttf")
	mf, err := fac.CreateMemoryFonts([][]byte{arial, systemFont(t, "consola.ttf"), systemFont(t, "arialbd.ttf")})
	if err != nil {
		t.Fatal(err)
	}
	if n := mf.Collection.FamilyCount(); n != 2 {
		t.Errorf("FamilyCount %d, want 2 (Arial in two weights, Consolas)", n)
	}
	for _, name := range []string{"Arial", "Consolas"} {
		if _, ok, err := mf.Collection.FindFamilyName(name); err != nil || !ok {
			t.Errorf("FindFamilyName(%s) = %v, %v", name, ok, err)
		}
	}
	if n := mf.loader.FileCount(); n != 3 {
		t.Errorf("FileCount %d, want 3", n)
	}
	tf, err := fac.CreateTextFormat("Consolas", mf.Collection, FONT_WEIGHT_NORMAL, FONT_STYLE_NORMAL,
		FONT_STRETCH_NORMAL, 13, "en-us")
	if err != nil {
		t.Fatal(err)
	}
	// "iiii" and "WWWW" are equally wide in Consolas from the collection,
	// not in a proportional fallback.
	var ws [2]float32
	for i, s := range []string{"iiii", "WWWW"} {
		l, err := fac.CreateTextLayout(u16(s), tf, 1000, 500)
		if err != nil {
			t.Fatal(err)
		}
		m, err := l.Metrics()
		if err != nil {
			t.Fatal(err)
		}
		ws[i] = m.Width
		release(t, s, &l.Ptr, 0)
	}
	if ws[0] != ws[1] || ws[0] <= 0 {
		t.Errorf("Consolas widths %v, want equal", ws)
	}
	release(t, "format", &tf.Ptr, 0)
	mf.Release()
	mf.Release() // a second Release does nothing
	(*MemoryFonts)(nil).Release()
	if n := fac.RefCount(); n != start {
		t.Errorf("factory count %d, want %d at the start", n, start)
	}

	garbage := make([]byte, 4096)
	for i := range garbage {
		garbage[i] = byte(i * 7)
	}
	for _, c := range []struct {
		name  string
		files [][]byte
		want  error // nil: any error
	}{
		{"no files", nil, ErrNoFonts},
		{"garbage", [][]byte{garbage}, DWRITE_E_FILEFORMAT},
		{"one byte", [][]byte{{0}}, DWRITE_E_FILEFORMAT},
		{"truncated", [][]byte{arial[:64]}, nil},
		{"empty file", [][]byte{arial, {}}, nil},
		{"good then garbage", [][]byte{arial, garbage}, DWRITE_E_FILEFORMAT},
	} {
		mf, err := fac.CreateMemoryFonts(c.files)
		t.Logf("%s: %v", c.name, err)
		if err == nil {
			t.Errorf("%s: no error", c.name)
			mf.Release()
		} else if c.want != nil && !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
		if mf != nil {
			t.Errorf("%s: MemoryFonts %p with the error", c.name, mf)
		}
		if n := fac.RefCount(); n != start {
			t.Errorf("%s: factory count %d, want %d", c.name, n, start)
		}
	}

	// A corrupt file's Analyze says unsupported without an error.
	loader, err := fac.CreateInMemoryFontFileLoader()
	if err != nil {
		t.Fatal(err)
	}
	if err := fac.RegisterFontFileLoader(loader); err != nil {
		t.Fatal(err)
	}
	file, err := loader.CreateFontFileReference(fac, garbage)
	if err != nil {
		t.Fatal(err)
	}
	if a, err := file.Analyze(); err != nil || a.Supported {
		t.Errorf("garbage Analyze: %+v, %v; want unsupported, no error", a, err)
	}
	release(t, "garbage file", &file.Ptr, 0)
	if err := fac.UnregisterFontFileLoader(loader); err != nil {
		t.Error(err)
	}
	release(t, "loader", &loader.Ptr, 0)
	if n := fac.RefCount(); n != start {
		t.Errorf("factory count %d, want %d at the start", n, start)
	}
}

// warmUp runs an in-memory collection through a layout once, so the
// shared factory's process-wide caches exist before a test counts the
// factory's references: the first such run leaves three references held
// by DirectWrite itself (measured on Windows 11), every later run none.
func warmUp(t *testing.T, fac Factory) {
	t.Helper()
	mf, err := fac.CreateMemoryFonts([][]byte{systemFont(t, "arial.ttf")})
	if err != nil {
		t.Fatal(err)
	}
	sys, err := fac.SystemFontCollection(false)
	if err != nil {
		t.Fatal(err)
	}
	width(t, fac, "Arial", mf.Collection, "warm")
	width(t, fac, "Arial", sys, "warm")
	sys.Release()
	mf.Release()
}
