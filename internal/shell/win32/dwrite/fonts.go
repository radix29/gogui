//go:build windows

package dwrite

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/radix29/gogui/internal/shell/win32/com"
	"golang.org/x/sys/windows"
)

// Interface IDs of the in-memory font path (S2.5c).
var (
	IID_IDWriteFontFileLoader         = com.MustGUID("{727CAD4E-D6AF-4C9E-8A08-D695B11CAA49}")
	IID_IDWriteInMemoryFontFileLoader = com.MustGUID("{DC102F47-A12D-4B1C-822D-9E117E33043F}")
	IID_IDWriteFontFile               = com.MustGUID("{739D886A-CEF5-47DC-8769-1A8B41BEBBB0}")
	IID_IDWriteFontSetBuilder         = com.MustGUID("{2F642AFE-9C68-4F40-B8BE-457401AFCB3D}")
	IID_IDWriteFontSetBuilder1        = com.MustGUID("{3FF7715F-3CDC-4DC6-9B72-EC5621DCCAFD}")
	IID_IDWriteFontSet                = com.MustGUID("{53585141-D9F8-4095-8321-D73CF6BD116B}")
	IID_IDWriteFontCollection1        = com.MustGUID("{53585141-D9F8-4095-8321-D73CF6BD116C}")
)

// Vtable slots of the in-memory font path, in dwrite.h/dwrite_3.h order.
const (
	// IDWriteFactory.
	slotFactoryRegisterFontFileLoader   = 13
	slotFactoryUnregisterFontFileLoader = 14
	// IDWriteFactory3.
	slotFactoryCreateFontCollectionFromFontSet = 37
	// IDWriteFactory5.
	slotFactoryCreateFontSetBuilder5        = 43 // the IDWriteFontSetBuilder1 overload
	slotFactoryCreateInMemoryFontFileLoader = 44

	// IDWriteFontCollection.
	slotCollectionFindFamilyName = 5

	// IDWriteInMemoryFontFileLoader (after IDWriteFontFileLoader 0–3).
	slotMemLoaderCreateInMemoryFontFileReference = 4
	slotMemLoaderGetFileCount                    = 5

	// IDWriteFontFile.
	slotFontFileAnalyze = 5

	// IDWriteFontSetBuilder1 (after IDWriteFontSetBuilder 0–6).
	slotBuilderCreateFontSet = 6
	slotBuilderAddFontFile   = 7

	// IDWriteFontSet.
	slotFontSetGetFontCount = 3
)

// DWRITE_E_FILEFORMAT is DirectWrite's answer for font data in a format it
// doesn't know (AddFontFile on bytes that are no font).
const DWRITE_E_FILEFORMAT = com.DWRITE_E_FILEFORMAT

// ErrNoFonts is the error of CreateMemoryFonts when the data holds no font
// DirectWrite can use.
var ErrNoFonts = errors.New("dwrite: no usable font in the data")

// CreateInMemoryFontFileLoader returns a new in-memory font file loader. It
// has to be registered (RegisterFontFileLoader) before files are made with
// it, and unregistered before its last release.
func (f Factory) CreateInMemoryFontFileLoader() (InMemoryFontFileLoader, error) {
	out, err := f.CallOut("IDWriteFactory5::CreateInMemoryFontFileLoader", slotFactoryCreateInMemoryFontFileLoader)
	return InMemoryFontFileLoader{out}, err
}

// RegisterFontFileLoader registers a loader with the factory, which holds a
// reference to it until UnregisterFontFileLoader. Registering it twice
// fails with DWRITE_E_ALREADYREGISTERED.
func (f Factory) RegisterFontFileLoader(l InMemoryFontFileLoader) error {
	return f.CallHR("IDWriteFactory::RegisterFontFileLoader", slotFactoryRegisterFontFileLoader, uintptr(l.Ptr))
}

// UnregisterFontFileLoader drops the factory's reference to a registered
// loader. Only call it once every collection, format and layout using the
// loader's files is released: DirectWrite reopens the files through the
// registered loader when it needs glyphs, and a layout after the loader is
// unregistered kills the process (measured on Windows 11).
func (f Factory) UnregisterFontFileLoader(l InMemoryFontFileLoader) error {
	return f.CallHR("IDWriteFactory::UnregisterFontFileLoader", slotFactoryUnregisterFontFileLoader, uintptr(l.Ptr))
}

// CreateFontSetBuilder returns an empty font set builder (the
// IDWriteFontSetBuilder1 overload of IDWriteFactory5, which has AddFontFile).
func (f Factory) CreateFontSetBuilder() (FontSetBuilder, error) {
	out, err := f.CallOut("IDWriteFactory5::CreateFontSetBuilder", slotFactoryCreateFontSetBuilder5)
	return FontSetBuilder{out}, err
}

// CreateFontCollectionFromFontSet groups a font set's fonts into families.
// The result is an IDWriteFontCollection1, usable wherever a FontCollection
// is (CreateTextFormat).
func (f Factory) CreateFontCollectionFromFontSet(s FontSet) (FontCollection, error) {
	out, err := f.CallOut("IDWriteFactory3::CreateFontCollectionFromFontSet", slotFactoryCreateFontCollectionFromFontSet,
		uintptr(s.Ptr))
	return FontCollection{out}, err
}

// FindFamilyName looks a family up by name (case-insensitive, in any of the
// font's localized names) and returns its index and whether it exists.
func (c FontCollection) FindFamilyName(name string) (index uint32, exists bool, err error) {
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, false, fmt.Errorf("dwrite: FindFamilyName %q: %w", name, err)
	}
	var found int32
	if err := c.CallHR("IDWriteFontCollection::FindFamilyName", slotCollectionFindFamilyName,
		uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&index)), uintptr(unsafe.Pointer(&found))); err != nil {
		return 0, false, err
	}
	if found == 0 {
		return 0, false, nil
	}
	return index, true, nil
}

// InMemoryFontFileLoader is IDWriteInMemoryFontFileLoader.
type InMemoryFontFileLoader struct{ com.Ptr }

// CreateFontFileReference makes a font file from data. No owner object is
// passed, so the loader copies the bytes: data may change or be collected
// right after the call, and no Go memory is held natively (PLAN.md §7.2).
// The bytes are not parsed here; data that is no font fails in
// FontSetBuilder.AddFontFile or FontFile.Analyze. Empty data is refused.
func (l InMemoryFontFileLoader) CreateFontFileReference(f Factory, data []byte) (FontFile, error) {
	if len(data) == 0 {
		return FontFile{}, errors.New("dwrite: CreateInMemoryFontFileReference: empty font data")
	}
	if uint64(len(data)) > 1<<32-1 {
		return FontFile{}, errors.New("dwrite: CreateInMemoryFontFileReference: font data over 4 GiB")
	}
	out, err := l.CallOut("IDWriteInMemoryFontFileLoader::CreateInMemoryFontFileReference", slotMemLoaderCreateInMemoryFontFileReference,
		uintptr(f.Ptr),
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0)
	return FontFile{out}, err
}

// FontFile is IDWriteFontFile.
type FontFile struct{ com.Ptr }

// FontFileAnalysis is the answer of FontFile.Analyze.
type FontFileAnalysis struct {
	Supported bool
	FileType  uint32 // DWRITE_FONT_FILE_TYPE (TRUETYPE 2, a collection 3)
	FaceType  uint32 // DWRITE_FONT_FACE_TYPE (TRUETYPE 1)
	FaceCount uint32
}

// FontSetBuilder is IDWriteFontSetBuilder1.
type FontSetBuilder struct{ com.Ptr }

// AddFontFile adds every face of a font file. Data that is no font fails
// with DWRITE_E_FILEFORMAT.
func (b FontSetBuilder) AddFontFile(f FontFile) error {
	return b.CallHR("IDWriteFontSetBuilder1::AddFontFile", slotBuilderAddFontFile, uintptr(f.Ptr))
}

// CreateFontSet returns a font set of the fonts added so far.
func (b FontSetBuilder) CreateFontSet() (FontSet, error) {
	out, err := b.CallOut("IDWriteFontSetBuilder::CreateFontSet", slotBuilderCreateFontSet)
	return FontSet{out}, err
}

// FontSet is IDWriteFontSet.
type FontSet struct{ com.Ptr }

// FontCount returns the number of fonts (faces) in the set.
func (s FontSet) FontCount() uint32 { return uint32(s.Call(slotFontSetGetFontCount)) }

// MemoryFonts is a font collection built from font files in memory, with
// the loader it needs. Release gives everything back.
type MemoryFonts struct {
	Collection FontCollection
	factory    Factory
	loader     InMemoryFontFileLoader
}

// CreateMemoryFonts builds one collection from the font files in files
// (TTF, OTF or TTC data; each is copied, see CreateFontFileReference). Any
// file DirectWrite can't use, or no font at all, is an error, and
// everything made so far is released. The MemoryFonts holds its own
// reference to the factory.
func (f Factory) CreateMemoryFonts(files [][]byte) (mf *MemoryFonts, err error) {
	if len(files) == 0 {
		return nil, ErrNoFonts
	}
	loader, err := f.CreateInMemoryFontFileLoader()
	if err != nil {
		return nil, err
	}
	if err := f.RegisterFontFileLoader(loader); err != nil {
		loader.Release()
		return nil, err
	}
	f.AddRef()
	m := &MemoryFonts{factory: f, loader: loader}
	defer func() {
		if err != nil {
			m.Release()
			mf = nil
		}
	}()

	builder, err := f.CreateFontSetBuilder()
	if err != nil {
		return nil, err
	}
	defer builder.Release()
	for i, data := range files {
		file, err := loader.CreateFontFileReference(f, data)
		if err != nil {
			return nil, fmt.Errorf("dwrite: font file %d: %w", i, err)
		}
		err = builder.AddFontFile(file)
		file.Release()
		if err != nil {
			return nil, fmt.Errorf("dwrite: font file %d: %w", i, err)
		}
	}
	set, err := builder.CreateFontSet()
	if err != nil {
		return nil, err
	}
	defer set.Release()
	if set.FontCount() == 0 {
		return nil, ErrNoFonts
	}
	m.Collection, err = f.CreateFontCollectionFromFontSet(set)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// Release releases the collection, unregisters and releases the loader and
// drops the factory reference. It must come after every format and layout
// made with the collection is released: DirectWrite reads the font files
// through the registered loader whenever it needs glyphs, and a layout
// after the loader is unregistered kills the process. Release on a nil or
// released MemoryFonts does nothing.
func (m *MemoryFonts) Release() {
	if m == nil || m.factory.IsNil() {
		return
	}
	m.Collection.Release()
	if !m.loader.IsNil() {
		_ = m.factory.UnregisterFontFileLoader(m.loader) // only fails for an unregistered loader
		m.loader.Release()
	}
	m.factory.Release()
}
