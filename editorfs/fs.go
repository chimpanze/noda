// Package editorfs provides access to the embedded editor frontend assets.
package editorfs

import "io/fs"

// FS holds the embedded editor frontend assets (rooted at dist/).
// It is nil when built without -tags embed_editor.
var FS fs.FS
