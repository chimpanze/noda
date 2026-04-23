package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeRedirectChain returns the URL of an HTTPS-style redirect chain handler.
// First request gets a 302 to the second; second returns 200 echoing the
// final Authorization header.
func makeRedirectChain(t *testing.T, redirectTarget string) (*httptest.Server, *httptest.Server) {
	t.Helper()
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Echo-Auth", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	target := redirectTarget
	if target == "" {
		target = final.URL
	}
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target)
		w.WriteHeader(http.StatusFound)
	}))
	return first, final
}

func TestRedirect_NoneReturns3xx(t *testing.T) {
	first, final := makeRedirectChain(t, "")
	defer first.Close()
	defer final.Close()

	check := buildCheckRedirect("none", 10)
	client := &http.Client{CheckRedirect: check, Timeout: 2 * time.Second}

	resp, err := client.Get(first.URL)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, final.URL, resp.Header.Get("Location"))
}

func TestRedirect_StripAuthRemovesHeaderCrossOrigin(t *testing.T) {
	first, final := makeRedirectChain(t, "")
	defer first.Close()
	defer final.Close()

	check := buildCheckRedirect("strip_auth", 10)
	client := &http.Client{CheckRedirect: check, Timeout: 2 * time.Second}

	req, _ := http.NewRequest("GET", first.URL, nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, "", resp.Header.Get("X-Echo-Auth"), "Authorization should be stripped on cross-origin redirect")
}

func TestRedirect_StripAuthKeepsHeaderSameOrigin(t *testing.T) {
	// Self-redirect on the same server: same-origin → keep header.
	srv := httptest.NewUnstartedServer(nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/end")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/end", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Echo-Auth", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	})
	srv.Config.Handler = mux
	srv.Start()
	defer srv.Close()

	check := buildCheckRedirect("strip_auth", 10)
	client := &http.Client{CheckRedirect: check, Timeout: 2 * time.Second}

	req, _ := http.NewRequest("GET", srv.URL+"/start", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, "Bearer secret", resp.Header.Get("X-Echo-Auth"))
}

func TestRedirect_SameOriginDeniesCrossOrigin(t *testing.T) {
	first, final := makeRedirectChain(t, "")
	defer first.Close()
	defer final.Close()

	check := buildCheckRedirect("same_origin", 10)
	client := &http.Client{CheckRedirect: check, Timeout: 2 * time.Second}

	_, err := client.Get(first.URL)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "cross-origin"), "got: %v", err)
}

func TestRedirect_MaxRedirectsCap(t *testing.T) {
	// Server that always redirects to itself.
	var u string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", u)
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()
	u = srv.URL

	check := buildCheckRedirect("strip_auth", 3)
	client := &http.Client{CheckRedirect: check, Timeout: 2 * time.Second}

	_, err := client.Get(srv.URL)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "max"), "got: %v", err)
}

func TestRedirect_StripAuthRemovesXKey(t *testing.T) {
	first, final := makeRedirectChain(t, "")
	defer first.Close()
	defer final.Close()

	// Echo any X-Api-Key header on the final hop.
	final.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Echo-Key", r.Header.Get("X-Api-Key"))
		w.WriteHeader(http.StatusOK)
	})

	check := buildCheckRedirect("strip_auth", 10)
	client := &http.Client{CheckRedirect: check, Timeout: 2 * time.Second}

	req, _ := http.NewRequest("GET", first.URL, nil)
	req.Header.Set("X-Api-Key", "k1")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, "", resp.Header.Get("X-Echo-Key"))
}

func TestSameOrigin_DefaultPortNormalisation(t *testing.T) {
	a, _ := url.Parse("http://example.com/foo")
	b, _ := url.Parse("http://example.com:80/bar")
	assert.True(t, sameOrigin(a, b))

	c, _ := url.Parse("https://example.com:443/bar")
	d, _ := url.Parse("https://example.com/bar")
	assert.True(t, sameOrigin(c, d))

	e, _ := url.Parse("http://example.com:8080/bar")
	f, _ := url.Parse("http://example.com/bar")
	assert.False(t, sameOrigin(e, f))
}
