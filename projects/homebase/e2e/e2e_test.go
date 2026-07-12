//go:build e2e

// Package e2e drives the full Homebase lifecycle against a running
// docker-compose stack (see run.sh). It is the acceptance gate for the
// foundation spec: docs/superpowers/specs/2026-07-07-homebase-foundation-design.md
package e2e

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
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

// loginOrSetup logs in as the admin, bootstrapping the account via /setup
// first when the stack is fresh — lets any test run standalone against a
// fresh stack instead of depending on TestHomebaseLifecycle's setup (#310).
func loginOrSetup(t *testing.T) *client {
	t.Helper()
	anon := &client{t: t}
	resp := anon.doJSON("POST", "/auth/login", map[string]string{
		"email": adminEmail, "password": adminPassword,
	})
	if resp.StatusCode == 401 {
		drainAndClose(resp)
		setupResp := anon.doJSON("POST", "/setup", map[string]string{
			"setup_token": setupToken, "email": adminEmail, "password": adminPassword,
		})
		// 201 on a fresh stack; 403 if something else completed setup first.
		if setupResp.StatusCode != 201 && setupResp.StatusCode != 403 {
			t.Fatalf("setup: status %d", setupResp.StatusCode)
		}
		drainAndClose(setupResp)
		resp = anon.doJSON("POST", "/auth/login", map[string]string{
			"email": adminEmail, "password": adminPassword,
		})
	}
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
		// #310: deliberate fixed sleep for a 1s TTL. If this ever flakes,
		// replace with a short poll-until-404 loop — never a bigger sleep.
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

// videoGrant decodes a LiveKit JWT's payload and returns its "video" claim.
func videoGrant(t *testing.T, jwt string) map[string]any {
	t.Helper()
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("not a JWT: %q", jwt)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode JWT payload: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("unmarshal JWT payload: %v", err)
	}
	video, _ := claims["video"].(map[string]any)
	if video == nil {
		t.Fatalf("JWT has no video grant: %v", claims)
	}
	return video
}

// TestRoomsLifecycle runs after TestHomebaseLifecycle (same stack, admin
// already exists) and walks the meetings/streaming API against the
// dev-mode LiveKit container.
func TestRoomsLifecycle(t *testing.T) {
	anon := &client{t: t}
	owner := loginOrSetup(t)

	t.Run("unauthenticated room create is 401", func(t *testing.T) {
		resp := anon.doJSON("POST", "/rooms", map[string]string{"type": "meeting"})
		wantStatus(t, resp, 401)
		drainAndClose(resp)
	})

	var meetRoom, meetGuestToken, meetLinkPath string
	t.Run("create a meeting room", func(t *testing.T) {
		resp := owner.doJSON("POST", "/rooms", map[string]string{"type": "meeting"})
		wantStatus(t, resp, 201)
		body := decode(t, resp)
		meetRoom, _ = body["room"].(string)
		meetGuestToken, _ = body["guest_token"].(string)
		if !strings.HasPrefix(meetRoom, "hb-meet-") {
			t.Fatalf("room = %q, want hb-meet-*", meetRoom)
		}
		if len(meetGuestToken) != 64 {
			t.Fatalf("guest_token length = %d, want 64", len(meetGuestToken))
		}
		if body["livekit_url"] == "" || body["type"] != "meeting" {
			t.Fatalf("bad create body: %v", body)
		}
		meetLinkPath = "/j/" + meetGuestToken
	})

	t.Run("rooms list shows the room and its link", func(t *testing.T) {
		resp := owner.do("GET", "/rooms", nil, "")
		wantStatus(t, resp, 200)
		body := decode(t, resp)
		rooms, _ := body["rooms"].([]any)
		foundRoom := false
		for _, r := range rooms {
			m, _ := r.(map[string]any)
			if m["name"] == meetRoom {
				foundRoom = true
			}
		}
		if !foundRoom {
			t.Fatalf("room %s not in list", meetRoom)
		}
		links, _ := body["links"].([]any)
		foundLink := false
		for _, l := range links {
			m, _ := l.(map[string]any)
			if m["room_name"] == meetRoom && m["token"] == meetGuestToken {
				foundLink = true
			}
		}
		if !foundLink {
			t.Fatalf("guest link for %s not in list", meetRoom)
		}
	})

	t.Run("guest joins the meeting via link", func(t *testing.T) {
		resp := anon.do("GET", meetLinkPath+"?name=Alice", nil, "")
		wantStatus(t, resp, 200)
		body := decode(t, resp)
		if body["type"] != "meeting" || body["room"] != meetRoom {
			t.Fatalf("join body: %v", body)
		}
		grant := videoGrant(t, body["token"].(string))
		if grant["room"] != meetRoom {
			t.Fatalf("grant.room = %v", grant["room"])
		}
		if grant["canPublish"] != true {
			t.Fatalf("meeting guest canPublish = %v, want true", grant["canPublish"])
		}
	})

	t.Run("owner token carries roomAdmin", func(t *testing.T) {
		resp := owner.do("POST", "/rooms/"+meetRoom+"/token", nil, "")
		wantStatus(t, resp, 200)
		body := decode(t, resp)
		grant := videoGrant(t, body["token"].(string))
		if grant["roomAdmin"] != true || grant["room"] != meetRoom {
			t.Fatalf("owner grant: %v", grant)
		}
	})

	t.Run("rotate link kills the old token", func(t *testing.T) {
		resp := owner.doJSON("POST", "/rooms/"+meetRoom+"/link", nil)
		wantStatus(t, resp, 201)
		body := decode(t, resp)
		newTok, _ := body["guest_token"].(string)
		if newTok == "" || newTok == meetGuestToken {
			t.Fatalf("rotate returned %q", newTok)
		}

		resp = anon.do("GET", meetLinkPath, nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)

		resp = anon.do("GET", "/j/"+newTok, nil, "")
		wantStatus(t, resp, 200)
		drainAndClose(resp)
		meetGuestToken = newTok
		meetLinkPath = "/j/" + newTok
	})

	t.Run("revoke link stops new joins", func(t *testing.T) {
		resp := owner.do("DELETE", "/rooms/"+meetRoom+"/link", nil, "")
		wantStatus(t, resp, 204)
		drainAndClose(resp)
		resp = anon.do("GET", meetLinkPath, nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)
	})

	var streamRoom string
	t.Run("stream guests are subscribe-only", func(t *testing.T) {
		resp := owner.doJSON("POST", "/rooms", map[string]string{"type": "stream"})
		wantStatus(t, resp, 201)
		body := decode(t, resp)
		streamRoom, _ = body["room"].(string)
		tok, _ := body["guest_token"].(string)
		if !strings.HasPrefix(streamRoom, "hb-stream-") {
			t.Fatalf("room = %q", streamRoom)
		}

		resp = anon.do("GET", "/j/"+tok+"?name=Bob", nil, "")
		wantStatus(t, resp, 200)
		joinBody := decode(t, resp)
		if joinBody["type"] != "stream" {
			t.Fatalf("type = %v", joinBody["type"])
		}
		grant := videoGrant(t, joinBody["token"].(string))
		if grant["canPublish"] != false {
			t.Fatalf("stream guest canPublish = %v, want false", grant["canPublish"])
		}
	})

	t.Run("expiring room link dies", func(t *testing.T) {
		resp := owner.doJSON("POST", "/rooms/"+streamRoom+"/link", map[string]string{"expires_in": "1s"})
		wantStatus(t, resp, 201)
		body := decode(t, resp)
		tok, _ := body["guest_token"].(string)
		// #310: deliberate fixed sleep for a 1s TTL. If this ever flakes,
		// replace with a short poll-until-404 loop — never a bigger sleep.
		time.Sleep(1500 * time.Millisecond)
		resp = anon.do("GET", "/j/"+tok, nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)
	})

	t.Run("unknown join token is the same 404", func(t *testing.T) {
		resp := anon.do("GET", "/j/"+strings.Repeat("e", 64), nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)
	})

	t.Run("deleting an already-gone room is 404 but harmless", func(t *testing.T) {
		resp := owner.do("DELETE", "/rooms/hb-meet-ffffffff", nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)
	})

	t.Run("delete room removes it and its links", func(t *testing.T) {
		resp := owner.doJSON("POST", "/rooms/"+streamRoom+"/link", nil)
		wantStatus(t, resp, 201)
		body := decode(t, resp)
		tok, _ := body["guest_token"].(string)

		resp = owner.do("DELETE", "/rooms/"+streamRoom, nil, "")
		wantStatus(t, resp, 204)
		drainAndClose(resp)

		resp = anon.do("GET", "/j/"+tok, nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)

		resp = owner.do("GET", "/rooms", nil, "")
		wantStatus(t, resp, 200)
		listBody := decode(t, resp)
		for _, r := range listBody["rooms"].([]any) {
			m, _ := r.(map[string]any)
			if m["name"] == streamRoom {
				t.Fatalf("deleted room %s still listed", streamRoom)
			}
		}
	})
}

// execPsql runs a statement in the e2e Postgres container (project name from
// e2e/run.sh). Returns combined output; fails the test on error when
// mustSucceed is true.
func execPsql(t *testing.T, sql string, mustSucceed bool) (string, error) {
	t.Helper()
	cmd := exec.Command("docker", "exec", "-i", "homebase-e2e-postgres-1",
		"psql", "-U", "noda", "-d", "noda", "-tAc", sql)
	out, err := cmd.CombinedOutput()
	if mustSucceed && err != nil {
		t.Fatalf("psql %q: %v\n%s", sql, err, out)
	}
	return string(out), err
}

// TestSingleAdminIndex proves the #304 migration: a second auth_users row is
// impossible at the database level, whatever the workflow does.
func TestSingleAdminIndex(t *testing.T) {
	_ = loginOrSetup(t) // guarantees exactly one admin row exists
	out, err := execPsql(t, `INSERT INTO auth_users (id, email, password_hash, status, roles, metadata, created_at, updated_at)
		VALUES ('race-probe', 'second@example.com', 'x', 'active', '[]', '{}', now(), now())`, false)
	if err == nil {
		execPsql(t, `DELETE FROM auth_users WHERE id = 'race-probe'`, true)
		t.Fatalf("second auth_users row accepted — single-admin index missing? out=%s", out)
	}
	if !strings.Contains(out, "auth_users_single_row") {
		t.Fatalf("insert failed for an unexpected reason: %s", out)
	}
}

// TestSetupRaceNeverTwoAccounts fires two concurrent /setup calls with
// different emails. On a fresh stack exactly one may win; on an
// already-initialized stack both lose. Either way: never two 201s.
func TestSetupRaceNeverTwoAccounts(t *testing.T) {
	codes := make(chan int, 2)
	// Note (#304 plan): client.do t.Fatalf's on transport errors; from a
	// spawned goroutine that only Goexits the goroutine. The deferred
	// sentinel below keeps the receive from hanging in that case.
	for i := 0; i < 2; i++ {
		go func() {
			code := -1
			defer func() { codes <- code }()
			anon := &client{t: t}
			resp := anon.doJSON("POST", "/setup", map[string]string{
				"setup_token": setupToken, "email": adminEmail, "password": adminPassword,
			})
			code = resp.StatusCode
			drainAndClose(resp)
		}()
	}
	a, b := <-codes, <-codes
	if a == 201 && b == 201 {
		t.Fatalf("both concurrent setups returned 201 — race not closed")
	}
}

func TestDropsCursorPagination(t *testing.T) {
	owner := loginOrSetup(t)

	t.Run("malformed before is 400", func(t *testing.T) {
		resp := owner.do("GET", "/drops?before=garbage", nil, "")
		wantStatus(t, resp, 400)
		drainAndClose(resp)
	})

	t.Run("same-timestamp rows page without skips", func(t *testing.T) {
		// 60 drops, then force one shared timestamp (only reachable via SQL —
		// the API always stamps now()).
		for i := 0; i < 60; i++ {
			resp := owner.doJSON("POST", "/drops", map[string]string{
				"text": fmt.Sprintf("cursor-probe-%02d", i),
			})
			wantStatus(t, resp, 201)
			drainAndClose(resp)
		}
		t.Cleanup(func() {
			execPsql(t, `DELETE FROM drops WHERE text LIKE 'cursor-probe-%'`, true)
		})
		execPsql(t, `UPDATE drops SET created_at = now() WHERE text LIKE 'cursor-probe-%'`, true)

		respBody := func(before, beforeID string) map[string]any {
			u := "/drops?q=cursor-probe"
			if before != "" {
				u += "&before=" + url.QueryEscape(before) + "&before_id=" + url.QueryEscape(beforeID)
			}
			resp := owner.do("GET", u, nil, "")
			wantStatus(t, resp, 200)
			return decode(t, resp)
		}

		first := respBody("", "")
		p1, _ := first["drops"].([]any)
		if len(p1) != 50 {
			t.Fatalf("page 1 = %d rows, want 50", len(p1))
		}
		nb, _ := first["next_before"].(string)
		nbID, _ := first["next_before_id"].(string)
		if nb == "" || nbID == "" {
			t.Fatalf("missing cursor: next_before=%q next_before_id=%q", nb, nbID)
		}

		page := func(before, beforeID string) []any {
			drops, _ := respBody(before, beforeID)["drops"].([]any)
			return drops
		}

		// Timestamp-only cursor documents the OLD bug: all 60 share one
		// timestamp, so strict created_at < ts returns nothing.
		if rest := page(nb, ""); len(rest) != 0 {
			t.Fatalf("timestamp-only page 2 = %d rows, want 0 (strict-< semantics)", len(rest))
		}

		// Tuple cursor gets the remaining 10, no dups.
		p2 := page(nb, nbID)
		if len(p2) != 10 {
			t.Fatalf("tuple page 2 = %d rows, want 10", len(p2))
		}
		seen := map[string]bool{}
		for _, raw := range append(append([]any{}, p1...), p2...) {
			row, _ := raw.(map[string]any)
			id, _ := row["id"].(string)
			if seen[id] {
				t.Fatalf("duplicate id across pages: %s", id)
			}
			seen[id] = true
		}
		if len(seen) != 60 {
			t.Fatalf("union = %d unique rows, want 60", len(seen))
		}
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
