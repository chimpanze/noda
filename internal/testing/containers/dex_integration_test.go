//go:build integration

package containers

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDexAuthCodeDance(t *testing.T) {
	issuer, clientID, clientSecret, redirectURI := StartDex(t)

	code := DexAuthCode(t, issuer, clientID, redirectURI)
	require.NotEmpty(t, code)

	// The code is real: redeem it at the token endpoint.
	resp, err := http.PostForm(issuer+"/token", url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, 200, resp.StatusCode)

	// Codes are single-use: a second dance yields a different code.
	code2 := DexAuthCode(t, issuer, clientID, redirectURI)
	assert.False(t, strings.EqualFold(code, code2))
}
