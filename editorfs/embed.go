//go:build embed_editor

package editorfs

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var raw embed.FS

func init() {
	sub, err := fs.Sub(raw, "dist")
	if err != nil {
		panic("editorfs: " + err.Error())
	}
	FS = sub
}
