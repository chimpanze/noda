package email

import (
	"crypto/rand"
	"fmt"
	"net/mail"

	"github.com/chimpanze/noda/pkg/api"
)

// maxEmailRecipients caps the combined to+cc+bcc count per message.
const maxEmailRecipients = 100

var emailServiceDeps = map[string]api.ServiceDep{
	"mailer": {Prefix: "email", Required: true},
}

// resolveRecipients resolves a field that can be a string or []string.
func resolveRecipients(nCtx api.ExecutionContext, config map[string]any, key string) ([]string, error) {
	raw, ok := config[key]
	if !ok {
		return nil, nil
	}

	var addrs []string
	switch v := raw.(type) {
	case string:
		val, err := nCtx.Resolve(v)
		if err != nil {
			return nil, fmt.Errorf("resolve %q: %w", key, err)
		}
		switch r := val.(type) {
		case string:
			addrs = []string{r}
		case []any:
			a, err := anySliceToStrings(r, key)
			if err != nil {
				return nil, err
			}
			addrs = a
		case []string:
			addrs = r
		default:
			return nil, fmt.Errorf("field %q resolved to %T, expected string or []string", key, val)
		}
	case []any:
		a, err := anySliceToStrings(v, key)
		if err != nil {
			return nil, err
		}
		addrs = a
	case []string:
		addrs = v
	default:
		return nil, fmt.Errorf("field %q has invalid type %T", key, raw)
	}

	for i, a := range addrs {
		if _, err := mail.ParseAddress(a); err != nil {
			return nil, &api.ValidationError{
				Field:   fmt.Sprintf("%s[%d]", key, i),
				Message: fmt.Sprintf("invalid email address %q: %v", a, err),
			}
		}
	}
	return addrs, nil
}

func anySliceToStrings(items []any, key string) ([]string, error) {
	result := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("field %q contains non-string element: %T", key, item)
		}
		result = append(result, s)
	}
	return result, nil
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
