package cookbook

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/migrate"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/server"
	"github.com/chimpanze/noda/pkg/api"
)

// Options configures a RunProject invocation. Env pairs are exported via
// t.Setenv before config load; MailpitAPI is the base URL mail steps poll.
type Options struct {
	Env        map[string]string
	MailpitAPI string
}

// RunProject loads the cookbook project at dir through the production config
// pipeline, boots the real server in-process, and replays the project's
// verify.json steps. Any failure fails t with the step name in the message.
//
// opts is variadic so pre-tranche-2 call sites (which pass no Options) keep
// compiling unchanged; at most one Options value is honored.
func RunProject(t *testing.T, dir string, plugins []api.Plugin, opts ...Options) {
	t.Helper()
	var opt Options
	if len(opts) > 0 {
		opt = opts[0]
	}
	// Capture whether the caller supplied Env before prepareEnv fills in a
	// default COOKBOOK_DATA_DIR — the deps guard needs to tell "no env
	// configured" (unit test calling RunProject directly) apart from "env
	// configured, just without deps-relevant vars" (the integration walker,
	// which always sets Env for service-backed families).
	hadEnv := opt.Env != nil
	if err := prepareEnv(t, dir, &opt); err != nil {
		t.Fatalf("cookbook %s: %v", filepath.Base(dir), err)
	}
	if err := runProject(dir, plugins, opt, hadEnv); err != nil {
		t.Fatalf("cookbook %s: %v", filepath.Base(dir), err)
	}
}

// runProjectRecorded runs the project and reports whether it failed, without
// failing t, along with the error message (empty on success). Used to test
// the runner itself against a project expected to be rejected (e.g.
// non-empty deps without Options.Env).
//
// runProject returns a plain error rather than calling t.Errorf/t.Fatalf so
// that this helper can observe failure without a t.Run sub-test: a failing
// sub-test marks the parent (and package) failed regardless of any recorded
// return value, which would make the expected-failure tests show up as a
// real `go test` failure.
func runProjectRecorded(t *testing.T, dir string, plugins []api.Plugin, opts ...Options) (bool, string) {
	t.Helper()
	var opt Options
	if len(opts) > 0 {
		opt = opts[0]
	}
	hadEnv := opt.Env != nil
	if err := prepareEnv(t, dir, &opt); err != nil {
		t.Logf("cookbook %s: %v", filepath.Base(dir), err)
		return true, err.Error()
	}
	if err := runProject(dir, plugins, opt, hadEnv); err != nil {
		t.Logf("cookbook %s: %v", filepath.Base(dir), err)
		return true, err.Error()
	}
	return false, ""
}

// prepareEnv exports Env + COOKBOOK_DATA_DIR and copies seed files.
func prepareEnv(t *testing.T, dir string, opt *Options) error {
	t.Helper()
	if _, ok := opt.Env["COOKBOOK_DATA_DIR"]; !ok {
		if opt.Env == nil {
			opt.Env = map[string]string{}
		}
		opt.Env["COOKBOOK_DATA_DIR"] = t.TempDir()
	}
	for k, v := range opt.Env {
		t.Setenv(k, v)
	}

	suite, err := LoadSuite(filepath.Join(dir, "verify.json"))
	if err != nil {
		return err
	}
	dataDir := opt.Env["COOKBOOK_DATA_DIR"]
	for dest, src := range suite.Seed {
		content, err := os.ReadFile(filepath.Join(dir, src))
		if err != nil {
			return fmt.Errorf("seed %q: %w", src, err)
		}
		target := filepath.Join(dataDir, dest)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("seed %q: %w", dest, err)
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return fmt.Errorf("seed %q: %w", dest, err)
		}
	}
	return nil
}

func runProject(dir string, plugins []api.Plugin, opt Options, hadEnv bool) error {
	suite, err := LoadSuite(filepath.Join(dir, "verify.json"))
	if err != nil {
		return err
	}
	if len(suite.Deps) > 0 && !hadEnv {
		return fmt.Errorf("deps %v declared but no environment provided (run via the integration walker)", suite.Deps)
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

	if fi, err := os.Stat(filepath.Join(dir, "migrations")); err == nil && fi.IsDir() {
		svc, ok := boot.Services.Get("main-db")
		if !ok {
			return fmt.Errorf("migrations/ present but no main-db service")
		}
		gdb, ok := svc.(*gorm.DB)
		if !ok {
			return fmt.Errorf("main-db service is %T, not *gorm.DB", svc)
		}
		if _, err := migrate.Up(gdb, filepath.Join(dir, "migrations")); err != nil {
			return fmt.Errorf("migrations: %w", err)
		}
	}

	// The cache must be built here and injected via WithWorkflowCache: although
	// srv.Setup() would self-build an identical cache, NewServer only wires the
	// sub-workflow runner (used by workflow.run / control.loop) when the cache
	// is already present at construction time (server.go NewServer).
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
		if step.Mail != nil {
			if opt.MailpitAPI == "" {
				return fmt.Errorf("step %q: mail step but no MailpitAPI configured", step.Name)
			}
			if err := checkMail(opt.MailpitAPI, *step.Mail); err != nil {
				return fmt.Errorf("step %q: %w", step.Name, err)
			}
			continue
		}
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
	contentType := ""
	if hasBody {
		raw, err := json.Marshal(substituteBody(step.Request.Body, vars))
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
		contentType = "application/json"
	}
	if step.Request.Multipart != nil {
		ct, buf, err := buildMultipart(step.Request.Multipart, vars)
		if err != nil {
			return fmt.Errorf("multipart: %w", err)
		}
		bodyReader = buf
		contentType = ct
		hasBody = true
	}

	req := httptest.NewRequest(step.Request.Method, path, bodyReader)
	if hasBody {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range step.Request.Headers {
		req.Header.Set(k, Substitute(v, vars))
	}

	resp, err := srv.App().Test(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != step.Expect.Status {
		return fmt.Errorf("expected status %d, got %d (body: %.500s)", step.Expect.Status, resp.StatusCode, raw)
	}
	for k, want := range step.Expect.Headers {
		expected := Substitute(want, vars)
		if got := resp.Header.Get(k); got != expected {
			return fmt.Errorf("expected header %s=%q, got %q", k, expected, got)
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

// buildMultipart renders a MultipartSpec into a body and content type, with
// ${var} substitution applied to field values and text file contents.
func buildMultipart(spec *MultipartSpec, vars map[string]string) (string, *bytes.Buffer, error) {
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	for name, val := range spec.Fields {
		if err := w.WriteField(name, Substitute(val, vars)); err != nil {
			return "", nil, err
		}
	}
	for _, f := range spec.Files {
		field := f.Field
		if field == "" {
			field = "file"
		}
		hdr := textproto.MIMEHeader{}
		hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, field, f.Filename))
		if f.ContentType != "" {
			hdr.Set("Content-Type", f.ContentType)
		}
		part, err := w.CreatePart(hdr)
		if err != nil {
			return "", nil, err
		}
		var data []byte
		if f.ContentBase64 != "" {
			data, err = base64.StdEncoding.DecodeString(f.ContentBase64)
			if err != nil {
				return "", nil, fmt.Errorf("file %q: %w", f.Filename, err)
			}
		} else {
			data = []byte(Substitute(f.Content, vars))
		}
		if _, err := part.Write(data); err != nil {
			return "", nil, err
		}
	}
	if err := w.Close(); err != nil {
		return "", nil, err
	}
	return w.FormDataContentType(), buf, nil
}

// checkMail polls the Mailpit list API for a message matching the
// expectation. Matching is deliberately restricted to the list endpoint
// (GET /api/v1/messages): fetching individual messages would let BodyRegex
// match the full body, but message bodies can contain raw control bytes
// that break JSON decoding of the per-message endpoint, so BodyRegex is
// matched against the list response's Snippet field instead. Mailpit's
// MessageSummary does include Snippet (a plain-text preview), but if a
// given Mailpit version ever omits or empties it, an empty Snippet simply
// never matches a non-empty BodyRegex — treated as "not yet delivered"
// rather than a hard error, so the poll keeps retrying until the deadline.
func checkMail(apiBase string, want MailExpect) error {
	type message struct {
		Subject string `json:"Subject"`
		Snippet string `json:"Snippet"`
		To      []struct {
			Address string `json:"Address"`
		} `json:"To"`
	}
	var last []message
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(apiBase + "/api/v1/messages")
		if err != nil {
			return fmt.Errorf("mailpit: %w", err)
		}
		var out struct {
			Messages []message `json:"messages"`
		}
		err = json.NewDecoder(resp.Body).Decode(&out)
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("mailpit response: %w", err)
		}
		last = out.Messages
		for _, m := range out.Messages {
			if m.Subject != want.Subject {
				continue
			}
			toMatch := false
			for _, to := range m.To {
				if to.Address == want.To {
					toMatch = true
				}
			}
			if !toMatch {
				continue
			}
			if want.BodyRegex != "" {
				re, err := regexp.Compile(want.BodyRegex)
				if err != nil {
					return fmt.Errorf("mail body_regex: %w", err)
				}
				if !re.MatchString(m.Snippet) {
					continue
				}
			}
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("no message to %q with subject %q found (inbox: %v)", want.To, want.Subject, last)
}
