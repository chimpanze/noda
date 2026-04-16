// Query-only Wasm helper module.
//
// Demonstrates the minimal shape of a stateless helper: `initialize` + `query`,
// no `tick` export, no `tick_rate` in config. Use this pattern when you just
// need a pure function Noda's expression engine doesn't provide.
package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/extism/go-pdk"
	"github.com/nodafw/noda-pdk-go/noda"
)

//go:wasmexport initialize
func initialize() int32 {
	if _, err := noda.GetInitInput(); err != nil {
		return noda.Fail(err)
	}
	return 0
}

//go:wasmexport query
func query() int32 {
	raw := pdk.Input()

	var req struct {
		Op         string `json:"op"`
		Value      string `json:"value"`
		Default    string `json:"default_cc"`
		KeepStart  int    `json:"keep_start"`
		KeepEnd    int    `json:"keep_end"`
		MaskChar   string `json:"mask_char"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return noda.Fail(fmt.Errorf("invalid query: %w", err))
	}

	switch req.Op {
	case "format_phone_e164":
		return noda.Output(map[string]any{"value": formatPhoneE164(req.Value, req.Default)})
	case "mask":
		if req.MaskChar == "" {
			req.MaskChar = "*"
		}
		return noda.Output(map[string]any{"value": mask(req.Value, req.KeepStart, req.KeepEnd, req.MaskChar)})
	default:
		return noda.Fail(fmt.Errorf("unknown op %q (expected \"format_phone_e164\" or \"mask\")", req.Op))
	}
}

func main() {}

// formatPhoneE164 strips non-digits and prepends a leading "+". A lone leading
// "00" is treated as an international prefix. If no country code is detectable
// and defaultCC is non-empty, the result is prefixed with defaultCC.
func formatPhoneE164(raw, defaultCC string) string {
	var digits strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	d := digits.String()
	if strings.HasPrefix(d, "00") {
		return "+" + d[2:]
	}
	if strings.HasPrefix(raw, "+") {
		return "+" + d
	}
	if defaultCC != "" {
		cc := strings.TrimPrefix(defaultCC, "+")
		return "+" + cc + strings.TrimPrefix(d, cc)
	}
	return "+" + d
}

// mask keeps the first keepStart and last keepEnd runes of s and replaces the
// middle with repeated mask runes. Rune-aware (doesn't split UTF-8).
func mask(s string, keepStart, keepEnd int, maskChar string) string {
	runes := []rune(s)
	n := len(runes)
	if keepStart < 0 {
		keepStart = 0
	}
	if keepEnd < 0 {
		keepEnd = 0
	}
	if keepStart+keepEnd >= n {
		return s
	}
	mr, _ := utf8.DecodeRuneInString(maskChar)
	if mr == utf8.RuneError {
		mr = '*'
	}
	middle := strings.Repeat(string(mr), n-keepStart-keepEnd)
	return string(runes[:keepStart]) + middle + string(runes[n-keepEnd:])
}
