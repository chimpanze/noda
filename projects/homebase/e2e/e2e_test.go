//go:build e2e

// Package e2e drives the full Homebase lifecycle against a running
// docker-compose stack (see run.sh). It is the acceptance gate for the
// foundation spec: docs/superpowers/specs/2026-07-07-homebase-foundation-design.md
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

var (
	baseURL    = envOr("BASE_URL", "http://localhost:3000")
	setupToken = envOr("SETUP_TOKEN", "e2e-setup-token")
)

const (
	adminEmail    = "admin@example.com"
	adminPassword = "correct horse battery staple"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// client is a tiny API client bound to one session token ("one machine").
type client struct {
	t     *testing.T
	token string
}

func (c *client) do(method, path string, body io.Reader, contentType string) *http.Response {
	c.t.Helper()
	req, err := http.NewRequest(method, baseURL+path, body)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func (c *client) doJSON(method, path string, payload any) *http.Response {
	c.t.Helper()
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			c.t.Fatalf("marshal: %v", err)
		}
		body = bytes.NewReader(b)
	}
	return c.do(method, path, body, "application/json")
}

func decode(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return m
}

func wantStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, want, b)
	}
}

func drainAndClose(resp *http.Response) {
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func login(t *testing.T) *client {
	t.Helper()
	anon := &client{t: t}
	resp := anon.doJSON("POST", "/auth/login", map[string]string{
		"email": adminEmail, "password": adminPassword,
	})
	wantStatus(t, resp, 200)
	body := decode(t, resp)
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatal("login returned no token")
	}
	return &client{t: t, token: token}
}

// TestHomebaseLifecycle is one ordered walk through the whole API. Subtests
// share state (the drops/shares created earlier) and must run in order.
func TestHomebaseLifecycle(t *testing.T) {
	anon := &client{t: t}

	// --- setup ---
	t.Run("setup with wrong token is 403", func(t *testing.T) {
		resp := anon.doJSON("POST", "/setup", map[string]string{
			"setup_token": "definitely-wrong", "email": adminEmail, "password": adminPassword,
		})
		wantStatus(t, resp, 403)
		drainAndClose(resp)
	})

	t.Run("setup creates the admin", func(t *testing.T) {
		resp := anon.doJSON("POST", "/setup", map[string]string{
			"setup_token": setupToken, "email": adminEmail, "password": adminPassword,
		})
		wantStatus(t, resp, 201)
		drainAndClose(resp)
	})

	t.Run("second setup is 403 even with the right token", func(t *testing.T) {
		resp := anon.doJSON("POST", "/setup", map[string]string{
			"setup_token": setupToken, "email": "evil@example.com", "password": "hunter2hunter2",
		})
		wantStatus(t, resp, 403)
		drainAndClose(resp)
	})

	// --- auth ---
	t.Run("unauthenticated request is 401", func(t *testing.T) {
		resp := anon.do("GET", "/drops", nil, "")
		wantStatus(t, resp, 401)
		drainAndClose(resp)
	})

	machineA := login(t) // "laptop"
	machineB := login(t) // "desktop"

	t.Run("me returns the admin", func(t *testing.T) {
		resp := machineA.do("GET", "/auth/me", nil, "")
		wantStatus(t, resp, 200)
		body := decode(t, resp)
		if body["email"] != adminEmail {
			t.Fatalf("me.email = %v", body["email"])
		}
	})

	// --- text drops & todos ---
	var todoDropID string
	t.Run("create a todo text drop", func(t *testing.T) {
		resp := machineA.doJSON("POST", "/drops", map[string]string{"text": "- [ ] buy milk"})
		wantStatus(t, resp, 201)
		body := decode(t, resp)
		todoDropID, _ = body["id"].(string)
		if todoDropID == "" {
			t.Fatal("no drop id")
		}
	})

	t.Run("the other machine sees the drop", func(t *testing.T) {
		resp := machineB.do("GET", "/drops", nil, "")
		wantStatus(t, resp, 200)
		body := decode(t, resp)
		drops, _ := body["drops"].([]any)
		if len(drops) != 1 {
			t.Fatalf("drops = %d, want 1", len(drops))
		}
	})

	t.Run("tick the checkbox via PATCH", func(t *testing.T) {
		resp := machineB.doJSON("PATCH", "/drops/"+todoDropID, map[string]string{"text": "- [x] buy milk"})
		wantStatus(t, resp, 200)
		body := decode(t, resp)
		if body["text"] != "- [x] buy milk" {
			t.Fatalf("text = %v", body["text"])
		}
	})

	t.Run("search finds by content", func(t *testing.T) {
		resp := machineA.do("GET", "/drops?q=milk", nil, "")
		wantStatus(t, resp, 200)
		body := decode(t, resp)
		if drops, _ := body["drops"].([]any); len(drops) != 1 {
			t.Fatalf("q=milk found %d drops, want 1", len(drops))
		}
		resp = machineA.do("GET", "/drops?q=zzz-not-there", nil, "")
		wantStatus(t, resp, 200)
		body = decode(t, resp)
		if drops, _ := body["drops"].([]any); len(drops) != 0 {
			t.Fatalf("q=zzz found %d drops, want 0", len(drops))
		}
	})

	// --- file drops ---
	fileContent := []byte("e2e file payload " + time.Now().Format(time.RFC3339Nano))
	var fileDropID string
	t.Run("upload a file drop with a comment", func(t *testing.T) {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		if err := w.WriteField("text", "the report"); err != nil {
			t.Fatal(err)
		}
		fw, err := w.CreateFormFile("file", "report.txt")
		if err != nil {
			t.Fatal(err)
		}
		fw.Write(fileContent)
		w.Close()

		resp := machineA.do("POST", "/drops/upload", &buf, w.FormDataContentType())
		wantStatus(t, resp, 201)
		body := decode(t, resp)
		fileDropID, _ = body["id"].(string)
		if fileDropID == "" {
			t.Fatal("no drop id")
		}
		if body["file_name"] != "report.txt" {
			t.Fatalf("file_name = %v", body["file_name"])
		}
	})

	t.Run("download the file from the other machine", func(t *testing.T) {
		resp := machineB.do("GET", "/drops/"+fileDropID+"/file", nil, "")
		wantStatus(t, resp, 200)
		defer resp.Body.Close()
		got, _ := io.ReadAll(resp.Body)
		if !bytes.Equal(got, fileContent) {
			t.Fatalf("downloaded bytes differ: got %q", got)
		}
		if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "report.txt") {
			t.Fatalf("Content-Disposition = %q", cd)
		}
	})

	// --- sharing ---
	var shareURL, shareID string
	t.Run("create a share link", func(t *testing.T) {
		resp := machineA.doJSON("POST", "/drops/"+fileDropID+"/share", nil)
		wantStatus(t, resp, 201)
		body := decode(t, resp)
		token, _ := body["token"].(string)
		shareID, _ = body["id"].(string)
		if token == "" || shareID == "" {
			t.Fatalf("share missing token/id: %v", body)
		}
		shareURL = "/s/" + token
	})

	t.Run("friend fetches the share unauthenticated", func(t *testing.T) {
		resp := anon.do("GET", shareURL, nil, "")
		wantStatus(t, resp, 200)
		body := decode(t, resp)
		if body["file_name"] != "report.txt" || body["has_file"] != true {
			t.Fatalf("share body = %v", body)
		}
		resp = anon.do("GET", shareURL+"/file", nil, "")
		wantStatus(t, resp, 200)
		defer resp.Body.Close()
		got, _ := io.ReadAll(resp.Body)
		if !bytes.Equal(got, fileContent) {
			t.Fatal("shared download differs")
		}
	})

	t.Run("owner lists active share links", func(t *testing.T) {
		resp := machineA.do("GET", "/shares", nil, "")
		wantStatus(t, resp, 200)
		body := decode(t, resp)
		shares, _ := body["shares"].([]any)
		found := false
		for _, s := range shares {
			m, _ := s.(map[string]any)
			if m["id"] == shareID {
				found = true
				if m["drop_id"] != fileDropID {
					t.Fatalf("share drop_id = %v, want %v", m["drop_id"], fileDropID)
				}
				if m["file_name"] != "report.txt" {
					t.Fatalf("share file_name = %v", m["file_name"])
				}
			}
		}
		if !found {
			t.Fatalf("created share %s not in /shares list (%d shares)", shareID, len(shares))
		}
	})

	t.Run("expiring link dies", func(t *testing.T) {
		resp := machineA.doJSON("POST", "/drops/"+todoDropID+"/share", map[string]string{"expires_in": "1s"})
		wantStatus(t, resp, 201)
		body := decode(t, resp)
		tok, _ := body["token"].(string)
		time.Sleep(1500 * time.Millisecond)
		resp = anon.do("GET", "/s/"+tok, nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)
	})

	t.Run("revoked link dies", func(t *testing.T) {
		resp := machineA.do("DELETE", "/shares/"+shareID, nil, "")
		wantStatus(t, resp, 204)
		drainAndClose(resp)
		resp = anon.do("GET", shareURL, nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)
	})

	t.Run("unknown share token is the same 404", func(t *testing.T) {
		resp := anon.do("GET", "/s/"+strings.Repeat("f", 64), nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)
	})

	// --- per-device session revocation ---
	t.Run("revoke machine B's session from machine A", func(t *testing.T) {
		resp := machineB.do("GET", "/auth/sessions", nil, "")
		wantStatus(t, resp, 200)
		body := decode(t, resp)
		bSession, _ := body["current_session_id"].(string)
		if bSession == "" {
			t.Fatal("no current_session_id")
		}
		sessions, _ := body["sessions"].([]any)
		if len(sessions) < 2 {
			t.Fatalf("sessions = %d, want >= 2", len(sessions))
		}

		resp = machineA.do("DELETE", "/auth/sessions/"+bSession, nil, "")
		wantStatus(t, resp, 204)
		drainAndClose(resp)

		resp = machineB.do("GET", "/drops", nil, "")
		wantStatus(t, resp, 401)
		drainAndClose(resp)
	})

	// --- deletion ---
	t.Run("delete the file drop; share links and file access die with it", func(t *testing.T) {
		// a fresh share link that must die with the drop (cascade)
		resp := machineA.doJSON("POST", "/drops/"+fileDropID+"/share", nil)
		wantStatus(t, resp, 201)
		body := decode(t, resp)
		tok, _ := body["token"].(string)

		resp = machineA.do("DELETE", "/drops/"+fileDropID, nil, "")
		wantStatus(t, resp, 204)
		drainAndClose(resp)

		resp = machineA.do("GET", "/drops/"+fileDropID, nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)
		resp = machineA.do("GET", "/drops/"+fileDropID+"/file", nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)
		resp = anon.do("GET", "/s/"+tok, nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)
	})

	// --- logout ---
	t.Run("logout kills machine A's session", func(t *testing.T) {
		resp := machineA.do("POST", "/auth/logout", nil, "")
		wantStatus(t, resp, 204)
		drainAndClose(resp)
		resp = machineA.do("GET", "/drops", nil, "")
		wantStatus(t, resp, 401)
		drainAndClose(resp)
	})
}

func TestMain(m *testing.M) {
	// Wait for the stack (run.sh also waits; this is belt & braces).
	deadline := time.Now().Add(60 * time.Second)
	for {
		resp, err := http.Get(baseURL + "/health/ready")
		if err == nil {
			drainAndClose(resp)
			if resp.StatusCode == 200 {
				break
			}
		}
		if time.Now().After(deadline) {
			fmt.Fprintln(os.Stderr, "homebase stack not ready at", baseURL)
			os.Exit(1)
		}
		time.Sleep(time.Second)
	}
	os.Exit(m.Run())
}
