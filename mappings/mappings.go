// Package mappings embeds the Karabiner keymap JSON so it can be parsed at
// runtime on any build target — including GOOS=js/wasm, which has no
// filesystem — without reading from disk. The embedded FS satisfies io/fs.FS,
// so core.ParseMapping can consume it directly.
package mappings

import "embed"

// FS holds the bundled mapping sets, one directory per set.
//
//go:embed qwerty-mirror/*.json
var FS embed.FS

// Default is the directory name of the current default mapping set within FS.
const Default = "qwerty-mirror"
