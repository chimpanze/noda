// Package schemas embeds JSON Schema definitions for all Noda config file types.
package schemas

import "embed"

//go:embed *.json
var FS embed.FS
