package storage

import "github.com/chimpanze/noda/pkg/api"

var storageDeps = map[string]api.ServiceDep{
	"storage": {Prefix: "storage", Required: true},
}
