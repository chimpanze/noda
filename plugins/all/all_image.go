//go:build !noimage

package all

import (
	imageplugin "github.com/chimpanze/noda/plugins/image"
)

func init() {
	optional = append(optional, &imageplugin.Plugin{})
}
