//go:build integration

package cookbook

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/chimpanze/noda/internal/testing/containers"
	"github.com/chimpanze/noda/pkg/api"
	cacheplugin "github.com/chimpanze/noda/plugins/cache"
	"github.com/chimpanze/noda/plugins/core/control"
	"github.com/chimpanze/noda/plugins/core/response"
	corestorage "github.com/chimpanze/noda/plugins/core/storage"
	"github.com/chimpanze/noda/plugins/core/transform"
	"github.com/chimpanze/noda/plugins/core/upload"
	"github.com/chimpanze/noda/plugins/core/util"
	workflowplugin "github.com/chimpanze/noda/plugins/core/workflow"
	dbplugin "github.com/chimpanze/noda/plugins/db"
	emailplugin "github.com/chimpanze/noda/plugins/email"
	imageplugin "github.com/chimpanze/noda/plugins/image"
	"github.com/stretchr/testify/require"
)

func cookbookPlugins() []api.Plugin {
	return []api.Plugin{
		&control.Plugin{},
		&transform.Plugin{},
		&response.Plugin{},
		&util.Plugin{},
		&workflowplugin.Plugin{},
		&dbplugin.Plugin{},
		&cacheplugin.Plugin{},
		&corestorage.Plugin{},
		&upload.Plugin{},
		&imageplugin.Plugin{},
		&emailplugin.Plugin{},
	}
}

// resolveDeps starts one container per declared dep and returns the runner
// options carrying the resulting environment.
func resolveDeps(t *testing.T, deps []string) Options {
	opt := Options{Env: map[string]string{}}
	for _, dep := range deps {
		switch dep {
		case "postgres":
			opt.Env["DATABASE_URL"] = containers.StartPostgres(t)
		case "redis":
			opt.Env["REDIS_URL"] = containers.StartRedis(t)
		case "mailpit":
			host, port, apiBase := containers.StartMailpit(t)
			opt.Env["SMTP_HOST"] = host
			opt.Env["SMTP_PORT"] = strconv.Itoa(port)
			opt.MailpitAPI = apiBase
		default:
			t.Fatalf("unknown cookbook dep %q", dep)
		}
	}
	return opt
}

func TestCookbook(t *testing.T) {
	dirs, err := filepath.Glob("../../../examples/node-cookbook/*")
	require.NoError(t, err)

	ran := 0
	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, "verify.json")); err != nil {
			continue
		}
		ran++
		t.Run(filepath.Base(dir), func(t *testing.T) {
			suite, err := LoadSuite(filepath.Join(dir, "verify.json"))
			require.NoError(t, err)
			opts := []Options{}
			if len(suite.Deps) > 0 {
				opts = append(opts, resolveDeps(t, suite.Deps))
			}
			RunProject(t, dir, cookbookPlugins(), opts...)
		})
	}
	require.NotZero(t, ran, "no cookbook projects found — wrong path?")
}
