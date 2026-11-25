# Review plan: open items (review of 2026-11-20)

The open items of the whole-project review after S6.3. Done items are removed; the commits
(W1–W17, W19, W20), PLAN.md §11.1 phase R and ARCHITECTURE.md record what was built. IDs are stable
(gaps are done items). Backward compatibility may be broken (user, 2026-11-20).

**Rules.** Every item starts with a failing test (or, for perf, a benchmark or alloc test)
that reproduces it; no test, no item. Each step follows PLAN.md §11.1: `bash
scripts/check.sh` before ✅, ARCHITECTURE.md updated. Pinned `.states`/`.frames` dumps and
goldens stay byte-identical unless a step says otherwise.

## 1. Bugs and fake-shell divergences (gtk4)

Code for these is written and host-tested here; the Linux `native.sh` log closes them.

| ID | Where | Problem | Fix |
|---|---|---|---|
| B8 | gtk4 `canvas.go` | `render.Valid*` in the gtk4 canvas. **Code in (W7)**, host tests pass; Linux run open (L7). | — |
| B10 | gtk4 `surface.go` (`NewSurface` panic path) | A callback panicking during creation skips `releaseIM`, leaking the `GtkIMMulticontext` and its IBus proxy. | `w.destroy(); w.releaseIM()`. |
| B11 | gtk4 `surface.go` `paintInvalid` | A minimized surface still queues a draw and calls `frameTimer.Expect(1)`; the timing record never closes, `invalid` stays set. | `&& !sf.minimized`, as the fake and `tickWidget`. |
| B16 | gtk4 `portalclient.go` `apply`, `connectBus` | `sig.Sender` is unchecked, so any bus peer can send `SettingChanged`. A hung `dial` leaks a goroutine and an fd. | Compare with the portal's unique name (`GetNameOwner`/`NameOwnerChanged`); dial with a deadline. |
| V4, R6 | gtk4 | `shellkit.Timers`, `ClipboardText`, `FixedText` in gtk4. **Code in (W3)**; Linux run open (L3, incl. `TestAfterRealGTK`). | — |
| V5 | gtk4 `Close`/`Quit` inside `Resized` | gtk4 alone posts them, so the surface stays live afterwards. | Mark the surface dead and send `Closed` synchronously; post only the native destroy. |

## 2. Performance

| ID | Where | Cost | Fix |
|---|---|---|---|
| P6 | gtk4 `canvas.go` `DrawText`, `text_linux.go` `charBoxes` | A new PangoLayout per text per frame; 4 FFI calls per character for geometry. | Port win32's `textcache.go` LRU; iterate by cluster. |

## 3. Refactors

| ID | Area | Change |
|---|---|---|
| R10 | shell iface | `shell.Shell` simplified: `Caps()` (`PopupShadow`), the optional `PopupShell` instead of `ErrUnsupported`, `Surface.SetSizeLimits(min, max)`, `TextSystem`/`Clock` handed over once in `Handler.Ready(Services)`. **Code in (W19)** for the fake, win32 and gtk4, host tests pass; Linux run open (L19). |
| R13 | gtk4 | One GValue property reader (3 copies) and `disconnectAll` (2 copies). |

## 4. Dead code to remove or move into `export_test.go`

Grep each before removing.

- gtk4: `mappedResize`/`resizeSizeRequest` and the `forced*` fields, once the S4.3j Linux log settles the resize mode; `cssDisplays` → `sync.Once`.

## 5. Docs and scripts

| ID | Change |
|---|---|
| D7 | `native.sh`/`idlecpu.sh` hardening. **Done on windows/amd64 (W2)**; open: the Linux run (L2) and macOS bash 3.2 (M2). |

## 6. Remaining order and where each step is closed

W steps are implemented and verified completely on this Windows/amd64 host (`check.sh` plus
`native.sh`). An L or M step is implemented and passes `check.sh` here first, and is closed
when the user's log is back. The number is the step; keep the order.

**W (this host)**

None left (W1–W20 are done). Next is L8, the first L step whose code is not in yet.

**L (Linux, the user runs `bash scripts/native.sh` on X11 and Wayland)**

- L2. D7: `native.sh` end to end on Linux (xvfb and weston steps, no double build).
- L3. R6, V4: gtk4 on `shellkit.Timers`, clipboard sanitizing, fixed-width TextSystem (code in, W3).
- L4. The gtk4 tests confirm the fake's new ordering (quit-first, modality before `Closed`, asynchronous `Placed` on Wayland).
- L7. B8 in the gtk4 canvas (code in, W7).
- L8. B10, B11, B16, V5: gtk4 fixes.
- L18. P6, R13: gtk4 text cache, cluster iteration, GLib helpers.
- L19. R10 on gtk4 (code in, W19): the gtk4 Linux tests on `SetSizeLimits` (`TestSurfaceGTKMinSize`, its `RESIZE-RESULT` line, the closed-surface test), `Caps` and `Ready(Services)`.
- L20. The gtk4 dead code of §4, once the S4.3j log settles the resize mode.

**M (macOS, the user runs `bash scripts/native.sh`; headless driver, shared tests, ffi callback benchmark, `spike/appkit` self-test)**

- M2. D7: `native.sh` under macOS bash 3.2 (arrays, `trap`, `main | tee`), keeping the `spike/appkit` step.
- M9. A1–A9: the examples and gogui tests on the darwin headless driver (code in, W9).
- M10. R7 on darwin: `driver_darwin.go` returns `(Driver, error)` (code in, W10); one native run.
