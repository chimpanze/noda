package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServer(t *testing.T) {
	s := NewServer("1.0.0-test")
	require.NotNil(t, s)
}

func TestBuildNodeRegistry(t *testing.T) {
	nodeReg := buildNodeRegistry()

	types := nodeReg.AllTypes()
	assert.NotEmpty(t, types)

	// Verify key node types are registered
	expectedTypes := []string{
		"control.if",
		"control.switch",
		"control.loop",
		"db.query",
		"db.create",
		"db.find",
		"transform.set",
		"transform.map",
		"response.json",
		"util.log",
		"cache.get",
		"cache.set",
		// auth registers real nodes even though the runtime files it under
		// service-backed plugins — MCP discovery must see them (issue #327).
		"auth.create_user",
		"auth.get_user",
		"auth.verify_credentials",
		"auth.create_session",
		"auth.revoke_session",
		"auth.create_token",
		"auth.consume_token",
		"auth.set_password",
	}
	for _, expected := range expectedTypes {
		desc, ok := nodeReg.GetDescriptor(expected)
		assert.True(t, ok, "expected node type %q to be registered", expected)
		if ok {
			assert.NotEmpty(t, desc.Description(), "expected description for %q", expected)
		}
	}
}

func TestAllPlugins(t *testing.T) {
	plugins := allPlugins()
	assert.NotEmpty(t, plugins)

	// All plugins should have a non-empty prefix
	for _, p := range plugins {
		assert.NotEmpty(t, p.Prefix(), "plugin %q has empty prefix", p.Name())
	}
}
