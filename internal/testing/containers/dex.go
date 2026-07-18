//go:build integration

package containers

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	dockercontainer "github.com/moby/moby/api/types/container"
	dockernetwork "github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	dexClientID     = "cookbook-client"
	dexClientSecret = "cookbook-secret"
	dexRedirectURI  = "http://127.0.0.1:18888/callback"
	// bcrypt of "password" — Dex's canonical example hash.
	dexPasswordHash = "$2a$10$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W"
	dexEmail        = "admin@example.com"
	dexPassword     = "password"
)

// StartDex starts a Dex OIDC provider with a static client and password
// connector. The issuer embeds a pre-reserved fixed host port so OIDC
// discovery's issuer check passes from the host.
func StartDex(t testing.TB) (issuer, clientID, clientSecret, redirectURI string) {
	t.Helper()
	ctx := context.Background()

	// Reserve a free host port, then bind it explicitly (issuer must be known
	// before the container starts).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve dex port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	issuer = fmt.Sprintf("http://127.0.0.1:%d/dex", port)
	cfg := fmt.Sprintf(`issuer: %s
storage:
  type: memory
web:
  http: 0.0.0.0:5556
oauth2:
  skipApprovalScreen: true
  responseTypes: ["code"]
staticClients:
  - id: %s
    secret: %s
    name: Cookbook
    redirectURIs:
      - %s
enablePasswordDB: true
staticPasswords:
  - email: %s
    hash: "%s"
    username: admin
    userID: cookbook-user-1
`, issuer, dexClientID, dexClientSecret, dexRedirectURI, dexEmail, dexPasswordHash)

	cfgPath := filepath.Join(t.TempDir(), "dex.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write dex config: %v", err)
	}

	req := testcontainers.ContainerRequest{
		Image:        "ghcr.io/dexidp/dex:v2.41.1",
		ExposedPorts: []string{"5556/tcp"},
		Cmd:          []string{"dex", "serve", "/etc/dex/cfg/dex.yaml"},
		Files: []testcontainers.ContainerFile{{
			HostFilePath:      cfgPath,
			ContainerFilePath: "/etc/dex/cfg/dex.yaml",
			FileMode:          0o644,
		}},
		// ExposedPorts only declares container-side exposure; the fixed host
		// port (chosen above, embedded in the issuer URL) is bound explicitly
		// here since the issuer must be known before the container starts.
		HostConfigModifier: func(hc *dockercontainer.HostConfig) {
			hc.PortBindings = dockernetwork.PortMap{
				dockernetwork.MustParsePort("5556/tcp"): []dockernetwork.PortBinding{
					{HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: fmt.Sprintf("%d", port)},
				},
			}
		},
		WaitingFor: wait.ForHTTP("/dex/.well-known/openid-configuration").
			WithPort("5556/tcp").WithStartupTimeout(60 * time.Second),
	}
	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req, Started: true,
	})
	skipIfNoDocker(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	// Verify discovery serves the exact issuer we configured.
	resp, err := http.Get(issuer + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatalf("dex discovery: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var disc struct {
		Issuer string `json:"issuer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&disc); err != nil || disc.Issuer != issuer {
		t.Fatalf("dex discovery issuer mismatch: got %q want %q (err %v)", disc.Issuer, issuer, err)
	}
	return issuer, dexClientID, dexClientSecret, dexRedirectURI
}

// DexAuthCode drives Dex's password-connector login over plain HTTP and
// returns a fresh single-use authorization code (offline_access requested so
// the exchange yields a refresh_token).
//
// Verified empirically with TestDexAuthCodeDance against the pinned
// v2.41.1 image with skipApprovalScreen:true, the dance takes these hops
// (no separate /dex/approval hop is needed — that only applies with a
// consent screen enabled; the fallback below handles it defensively in
// case a future image reintroduces it):
//  1. GET  /dex/auth?...            -> 303 to /dex/auth/local?state=<reqID>
//  2. GET  /dex/auth/local?state=.. -> 200 HTML login form
//     (action="/dex/auth/local/login?back=&amp;state=<reqID>" — note the
//     HTML-entity-escaped "&amp;", which must be unescaped before use as a URL)
//  3. POST /dex/auth/local/login?back=&state=<reqID> (login, password)
//     -> 303 straight to redirectURI?code=...&state=...
//
// The client's CheckRedirect stops Go's http.Client from auto-following the
// final hop to redirectURI (which is not a served endpoint in tests), so the
// login POST's Location header is read directly rather than followed.
func DexAuthCode(t testing.TB, issuer, clientID, redirectURI string) string {
	t.Helper()

	// Client that never follows a redirect to the (unserved) redirect URI.
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if strings.HasPrefix(req.URL.String(), redirectURI) {
				return http.ErrUseLastResponse
			}
			return nil
		},
		Timeout: 15 * time.Second,
	}

	authURL := issuer + "/auth?" + url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {"openid profile email offline_access"},
		"state":         {"cookbook-dance"},
	}.Encode()

	resp, err := client.Get(authURL)
	if err != nil {
		t.Fatalf("dex auth request: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	// The login form posts back to /dex/auth/local/login?back=&state=... —
	// find the action (Dex may 302 straight to it; handle both by using the
	// final URL).
	loginURL := resp.Request.URL.String()
	if m := regexp.MustCompile(`action="([^"]+)"`).FindSubmatch(body); m != nil {
		// The HTML template renders query-string ampersands as the entity
		// "&amp;" — un-escape before treating the value as a URL, or the
		// literal "&amp;" ends up inside a single bogus query key.
		action := html.UnescapeString(string(m[1]))
		if strings.HasPrefix(action, "/") {
			base, _ := url.Parse(issuer)
			loginURL = base.Scheme + "://" + base.Host + action
		} else {
			loginURL = action
		}
	}

	resp, err = client.PostForm(loginURL, url.Values{"login": {dexEmail}, "password": {dexPassword}})
	if err != nil {
		t.Fatalf("dex login post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	loc := resp.Header.Get("Location")
	if loc == "" {
		loc = resp.Request.URL.String()
	}

	// Track the final response (resp or resp2) for error reporting.
	finalResp := resp

	// Defensive fallback: not hit against the pinned image (skipApprovalScreen
	// bypasses it), but in case a future Dex version routes through
	// /dex/approval before the redirect-URI code, follow it with one GET.
	if strings.Contains(loc, "/approval") {
		u, perr := url.Parse(loc)
		if perr == nil && !u.IsAbs() {
			base, _ := url.Parse(issuer)
			u = base.ResolveReference(u)
			loc = u.String()
		}
		resp2, err := client.Get(loc)
		if err != nil {
			t.Fatalf("dex approval get: %v", err)
		}
		defer func() { _ = resp2.Body.Close() }()
		finalResp = resp2
		loc = resp2.Header.Get("Location")
		if loc == "" {
			loc = resp2.Request.URL.String()
		}
	}

	u, err := url.Parse(loc)
	if err != nil || u.Query().Get("code") == "" {
		b, _ := io.ReadAll(finalResp.Body)
		t.Fatalf("dex dance did not yield a code (status %d, location %q, body %.300s)", finalResp.StatusCode, loc, b)
	}
	return u.Query().Get("code")
}
