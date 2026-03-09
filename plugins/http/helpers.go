package http

import "github.com/chimpanze/noda/pkg/api"

var httpServiceDeps = map[string]api.ServiceDep{
	"client": {Prefix: "http", Required: true},
}
