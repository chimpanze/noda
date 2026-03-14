// Package docs embeds documentation files for use by the MCP server and editor.
package docs

import "embed"

// FS embeds all documentation files (excluding _internal).
//
//go:embed 01-getting-started/*.md 02-config/*.md 03-nodes/*.md 04-guides/*.md 05-examples/*.md
var FS embed.FS
