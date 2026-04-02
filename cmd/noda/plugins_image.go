//go:build !noimage

package main

import (
	imageplugin "github.com/chimpanze/noda/plugins/image"
)

func init() {
	optionalPlugins = append(optionalPlugins, &imageplugin.Plugin{})
}
