package email

import (
	"crypto/rand"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

var emailServiceDeps = map[string]api.ServiceDep{
	"mailer": {Prefix: "email", Required: true},
}

func getEmailService(services map[string]any) (*Service, error) {
	svc, ok := services["mailer"]
	if !ok {
		return nil, fmt.Errorf("email service not configured")
	}
	es, ok := svc.(*Service)
	if !ok {
		return nil, fmt.Errorf("service does not implement email service")
	}
	return es, nil
}

func resolveString(nCtx api.ExecutionContext, config map[string]any, key string) (string, bool, error) {
	raw, ok := config[key]
	if !ok {
		return "", false, nil
	}
	expr, ok := raw.(string)
	if !ok {
		return "", false, fmt.Errorf("field %q must be a string", key)
	}
	val, err := nCtx.Resolve(expr)
	if err != nil {
		return "", false, fmt.Errorf("resolve %q: %w", key, err)
	}
	s, ok := val.(string)
	if !ok {
		return "", false, fmt.Errorf("field %q resolved to %T, expected string", key, val)
	}
	return s, true, nil
}

func resolveRequiredString(nCtx api.ExecutionContext, config map[string]any, key string) (string, error) {
	s, ok, err := resolveString(nCtx, config, key)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("missing required field %q", key)
	}
	return s, nil
}

// resolveRecipients resolves a field that can be a string or []string.
func resolveRecipients(nCtx api.ExecutionContext, config map[string]any, key string) ([]string, error) {
	raw, ok := config[key]
	if !ok {
		return nil, nil
	}

	switch v := raw.(type) {
	case string:
		val, err := nCtx.Resolve(v)
		if err != nil {
			return nil, fmt.Errorf("resolve %q: %w", key, err)
		}
		switch r := val.(type) {
		case string:
			return []string{r}, nil
		case []any:
			return anySliceToStrings(r, key)
		case []string:
			return r, nil
		}
		return nil, fmt.Errorf("field %q resolved to %T, expected string or []string", key, val)
	case []any:
		return anySliceToStrings(v, key)
	case []string:
		return v, nil
	}
	return nil, fmt.Errorf("field %q has invalid type %T", key, raw)
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
