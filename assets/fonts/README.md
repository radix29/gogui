# assets/fonts

Fonts embedded into gogui's drawn driver (PLAN.md §3.1, S2.5g). Every `.ttf` and `.otf`
file in this directory is compiled into the binary (package `fonts`, `go:embed`). The
Windows shell loads them into one in-memory DirectWrite collection; the Linux shell (GTK 4,
S4.2j) writes them once to `~/.cache/gogui/fonts/` (`os.UserCacheDir`, named by the
SHA-256 of their content) and adds them to Pango's font map through fontconfig. Only the
Windows and Linux shells import the package, so macOS binaries don't carry the fonts.

## What goes here

The UI font is **Inter** (SIL Open Font License 1.1; decision D9, PLAN.md §12, formerly open
question O1). Claude can only reach the Go module proxy, so the user adds the files
by hand from the Inter release (<https://rsms.me/inter/>, `Inter-4.x.zip`, folder
`extras/ttf/`):

| File | Face |
|---|---|
| `Inter-Regular.ttf` | regular (400) |
| `Inter-Italic.ttf` | italic (400) |
| `Inter-Medium.ttf` | medium (500) |
| `Inter-SemiBold.ttf` | semibold (600) |
| `Inter-Bold.ttf` | bold (700) |
| `OFL.txt` | the license, which must ship with the fonts |

Use the static text files (`Inter-*.ttf`), not `InterDisplay-*` and not the variable
`InterVariable*.ttf`: the family has to be called exactly `Inter`, which the text system
looks up for the default UI font. More weights or their italics may be added; file names
don't matter, only the family name inside the font.

## How it is used

On Windows:

* No font file here: the shell uses Segoe UI Variable Text (Windows 11) or Segoe UI.
* A file DirectWrite can't load (corrupt, truncated, not a font): the whole embedded
  collection is dropped and the shell falls back to Segoe UI as well. With
  `GOGUI_DEBUG=1` the reason is logged once.
* A named family (`render.Font.Family`) is looked up here first, then in the system fonts.

On Linux (S4.2j):

* The files are written to the cache once and reused while their content matches; a
  corrupt or foreign file there is replaced (written to a temporary name, then renamed).
* They are added to the font map before the first text is laid out, with
  `pango_font_map_add_font_file` (Pango 1.56+) or as fontconfig application fonts.
* No font file here, no writable cache, a file Pango or fontconfig refuses, or no family
  `Inter` afterwards: the shell uses the system UI font (the family of GTK's
  `gtk-font-name`, else `sans-serif`). With `GOGUI_DEBUG=1` the reason is logged once.
* The fonts are application fonts of the process: a named family is looked up among them
  and the system fonts alike.

Never put SF Pro, SF Symbols or any other Apple font or asset here (PLAN.md §3.1).
This README must stay: it makes the embed pattern match when there are no fonts.
