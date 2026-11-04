package gtk4

import "github.com/radix29/gogui/internal/ffi"

// wakeSource is one idle source the wake added: g_idle_add from the
// goroutine that queued work, then a Drain on the GTK thread. The queue
// coalesces wakes (one per Drain), so there is about one at a time. Each
// gets its own handle, which sourceFunc frees when the source has run.
type wakeSource struct {
	s  *Shell
	id uint32
	h  ffi.Handle
}

// wake is the queue's wake function (mainthread.Queue.SetWake): it adds
// one idle source that drains the queue. It may be called from any
// goroutine. After shutdown it does nothing. If GLib adds no source the
// wake is lost; g_idle_add only fails for an invalid function.
func (s *Shell) wake() {
	s.wakeMu.Lock()
	defer s.wakeMu.Unlock()
	if s.wakeClosed {
		return
	}
	w := &wakeSource{s: s}
	w.id, w.h = s.loop.idle(w, kindWake)
	if w.id == 0 {
		return
	}
	s.wakePending[w] = struct{}{}
	s.wakes.Add(1)
}

// source implements sourcePeer: the idle source drains the queue once and
// goes. A source that runs after shutdown (it was being added while the
// shell stopped) does nothing.
func (w *wakeSource) source(kind) bool {
	s := w.s
	s.wakeMu.Lock() // waits for wake to finish recording w
	_, live := s.wakePending[w]
	delete(s.wakePending, w)
	closed := s.wakeClosed
	s.wakeMu.Unlock()
	if !live || closed {
		return false
	}
	s.drains.Add(1)
	s.q.Drain()
	return false
}

// stopWakes ends the wake (shutdown): later wake calls do nothing, and the
// sources that have not run are removed and their handles freed.
func (s *Shell) stopWakes() {
	s.wakeMu.Lock()
	defer s.wakeMu.Unlock()
	s.wakeClosed = true
	for w := range s.wakePending {
		s.loop.remove(w.id)
		dispatch.Free(w.h)
	}
	clear(s.wakePending)
}
