package cookbook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/server"
	"github.com/chimpanze/noda/pkg/api"
)

// RunProject loads the cookbook project at dir through the production config
// pipeline, boots the real server in-process, and replays the project's
// verify.json steps. Any failure fails t with the step name in the message.
func RunProject(t *testing.T, dir string, plugins []api.Plugin) {
	t.Helper()
	if err := runProject(dir, plugins); err != nil {
		t.Fatalf("cookbook %s: %v", filepath.Base(dir), err)
	}
}

// runProjectRecorded runs the project and reports whether it failed, without
// failing t. Used to test the runner itself against a project expected to be
// rejected (e.g. non-empty deps).
//
// runProject returns a plain error rather than calling t.Errorf/t.Fatalf so
// that this helper can observe failure without a t.Run sub-test: a failing
// sub-test marks the parent (and package) failed regardless of any recorded
// return value, which would make TestRunProjectRejectsDeps's expected
// failure show up as a real `go test` failure.
func runProjectRecorded(t *testing.T, dir string, plugins []api.Plugin) bool {
	t.Helper()
	if err := runProject(dir, plugins); err != nil {
		t.Logf("cookbook %s: %v", filepath.Base(dir), err)
		return true
	}
	return false
}

func runProject(dir string, plugins []api.Plugin) error {
	suite, err := LoadSuite(filepath.Join(dir, "verify.json"))
	if err != nil {
		return err
	}
	if len(suite.Deps) > 0 {
		return fmt.Errorf("deps %v not supported in this tranche (containers arrive with the service families)", suite.Deps)
	}

	sm, err := config.NewSecretsManager(dir, "")
	if err != nil {
		return fmt.Errorf("secrets manager: %w", err)
	}
	rc, verrs := config.ValidateAll(dir, "", sm)
	if len(verrs) > 0 {
		return fmt.Errorf("config validation: %v", verrs)
	}

	preg := registry.NewPluginRegistry()
	for _, p := range plugins {
		if err := preg.Register(p); err != nil {
			return fmt.Errorf("registering plugin %s: %w", p.Name(), err)
		}
	}
	boot, berrs := registry.Bootstrap(context.Background(), rc, preg)
	if len(berrs) > 0 {
		return fmt.Errorf("bootstrap: %v", berrs)
	}

	// Build workflow cache from the resolved config
	wfCache, err := engine.NewWorkflowCache(rc.Workflows, boot.Nodes)
	if err != nil {
		return fmt.Errorf("workflow cache: %w", err)
	}

	srv, err := server.NewServer(rc, boot.Services, boot.Nodes, server.WithWorkflowCache(wfCache))
	if err != nil {
		return fmt.Errorf("server: %w", err)
	}
	if err := srv.Setup(); err != nil {
		return fmt.Errorf("server setup: %w", err)
	}

	vars := map[string]string{}
	for _, step := range suite.Steps {
		if err := runStep(srv, step, vars); err != nil {
			return fmt.Errorf("step %q: %w", step.Name, err)
		}
	}
	return nil
}

// substituteBody replaces ${name} refs in every string leaf of a decoded
// request body, so substituted values are JSON-escaped by the subsequent
// marshal rather than spliced into serialized text.
func substituteBody(v any, vars map[string]string) any {
	switch t := v.(type) {
	case string:
		return Substitute(t, vars)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = substituteBody(val, vars)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = substituteBody(val, vars)
		}
		return out
	default:
		return v
	}
}

func runStep(srv *server.Server, step Step, vars map[string]string) error {
	path := Substitute(step.Request.Path, vars)

	var bodyReader io.Reader
	hasBody := step.Request.Body != nil
	if hasBody {
		raw, err := json.Marshal(substituteBody(step.Request.Body, vars))
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	req := httptest.NewRequest(step.Request.Method, path, bodyReader)
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range step.Request.Headers {
		req.Header.Set(k, Substitute(v, vars))
	}

	resp, err := srv.App().Test(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != step.Expect.Status {
		return fmt.Errorf("expected status %d, got %d (body: %.500s)", step.Expect.Status, resp.StatusCode, raw)
	}
	for k, want := range step.Expect.Headers {
		if got := resp.Header.Get(k); got != Substitute(want, vars) {
			return fmt.Errorf("expected header %s=%q, got %q", k, want, got)
		}
	}
	if step.Expect.BodyText != nil {
		if string(raw) != *step.Expect.BodyText {
			return fmt.Errorf("expected body_text %q, got %q", *step.Expect.BodyText, raw)
		}
	}

	if len(step.Expect.Body) > 0 || len(step.Capture) > 0 {
		var doc any
		if err := json.Unmarshal(raw, &doc); err != nil {
			return fmt.Errorf("response is not JSON (%.200s): %w", raw, err)
		}
		for _, a := range step.Expect.Body {
			if err := CheckAssertion(doc, a); err != nil {
				return err
			}
		}
		if err := Capture(doc, step.Capture, vars); err != nil {
			return err
		}
	}
	return nil
}
