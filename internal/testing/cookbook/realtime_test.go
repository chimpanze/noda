package cookbook

import (
	"bufio"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSSEEvent(t *testing.T) {
	stream := ": heartbeat\n\ndata: {\"kind\":\"note\",\"n\":1}\n\n"
	r := bufio.NewReader(strings.NewReader(stream))

	evt, err := nextSSEData(r)
	require.NoError(t, err)
	assert.JSONEq(t, `{"kind":"note","n":1}`, evt)
}

func TestMatchMessage(t *testing.T) {
	ok, err := matchMessage([]byte(`{"type":"edit","user":"u1"}`), []BodyAssertion{
		{Path: "type", Equals: "edit"},
	})
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = matchMessage([]byte(`{"type":"user_joined"}`), []BodyAssertion{
		{Path: "type", Equals: "edit"},
	})
	require.NoError(t, err)
	assert.False(t, ok, "non-matching messages are skipped, not fatal")

	_, err = matchMessage([]byte(`not-json`), []BodyAssertion{{Path: "x", Exists: new(true)}})
	assert.Error(t, err)
}
