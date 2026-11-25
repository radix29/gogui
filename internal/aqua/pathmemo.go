package aqua

import "github.com/radix29/gogui/internal/render"

// pathMemo is a path an element builds in Paint, kept with the key it was
// built from (review P16): the same key returns the built path, a new one
// rebuilds it in the same storage. A *render.Path passed to a canvas
// escapes, so a path built afresh in every Paint costs allocations on
// every frame; a memo costs none once built. Canvases read a path during
// the draw call only, so the memo may be rebuilt for the next one.
type pathMemo[K comparable] struct {
	key   K
	built bool
	path  render.Path
}

// get returns the path for k, calling build on the emptied path first
// when k is not the key it was built from.
func (m *pathMemo[K]) get(k K, build func(p *render.Path)) *render.Path {
	if !m.built || m.key != k {
		m.path.Reset()
		build(&m.path)
		m.key, m.built = k, true
	}
	return &m.path
}

// scratch returns the path emptied, for a path that changes with every
// use (a menu's per-item marks): its storage is reused, so it allocates
// only while it grows.
func (m *pathMemo[K]) scratch() *render.Path {
	m.built = false
	m.path.Reset()
	return &m.path
}

// scratchPath is a pathMemo used through scratch only.
type scratchPath = pathMemo[struct{}]
