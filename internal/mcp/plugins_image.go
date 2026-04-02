//go:build !noimage

package mcp

import (
	imageplugin "github.com/chimpanze/noda/plugins/image"
)

func init() {
	optionalPlugins = append(optionalPlugins, &imageplugin.Plugin{})
}
