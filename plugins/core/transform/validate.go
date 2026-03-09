package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

type validateDescriptor struct{}

func (d *validateDescriptor) Name() string                           { return "validate" }
func (d *validateDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *validateDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"data":   map[string]any{"type": "string"},
			"schema": map[string]any{"type": "object"},
		},
		"required": []any{"schema"},
	}
}

type validateExecutor struct {
	compiled   *jsonschema.Schema
	compileErr error // stored if schema compilation failed at factory time
}

func newValidateExecutor(config map[string]any) api.NodeExecutor {
	schemaCfg, _ := config["schema"].(map[string]any)
	if schemaCfg == nil {
		return &validateExecutor{compileErr: fmt.Errorf("missing required field \"schema\"")}
	}

	// Compile the schema once at creation time
	c := jsonschema.NewCompiler()

	// Convert Go map to JSON and back for the schema compiler
	schemaBytes, err := json.Marshal(schemaCfg)
	if err != nil {
		return &validateExecutor{compileErr: fmt.Errorf("marshal schema: %w", err)}
	}

	var schemaDoc any
	if err := json.Unmarshal(schemaBytes, &schemaDoc); err != nil {
		return &validateExecutor{compileErr: fmt.Errorf("unmarshal schema: %w", err)}
	}

	if err := c.AddResource("schema.json", schemaDoc); err != nil {
		return &validateExecutor{compileErr: fmt.Errorf("add schema resource: %w", err)}
	}

	compiled, err := c.Compile("schema.json")
	if err != nil {
		return &validateExecutor{compileErr: fmt.Errorf("compile schema: %w", err)}
	}

	return &validateExecutor{compiled: compiled}
}

func (e *validateExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *validateExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	dataExpr, _ := config["data"].(string)
	if dataExpr == "" {
		dataExpr = "{{ input }}"
	}

	resolved, err := nCtx.Resolve(dataExpr)
	if err != nil {
		return "", nil, fmt.Errorf("transform.validate: data: %w", err)
	}

	if e.compiled == nil {
		return "", nil, fmt.Errorf("transform.validate: %w", e.compileErr)
	}

	// Validate the data - need to round-trip through JSON for proper type handling
	dataBytes, err := json.Marshal(resolved)
	if err != nil {
		return "", nil, fmt.Errorf("transform.validate: marshal data: %w", err)
	}

	var dataDoc any
	if err := json.Unmarshal(dataBytes, &dataDoc); err != nil {
		return "", nil, fmt.Errorf("transform.validate: unmarshal data: %w", err)
	}

	err = e.compiled.Validate(dataDoc)
	if err != nil {
		validationErr, ok := err.(*jsonschema.ValidationError)
		if ok {
			errors := collectValidationErrors(validationErr)
			return "", nil, &validationResultError{Errors: errors}
		}
		return "", nil, fmt.Errorf("transform.validate: %w", err)
	}

	return api.OutputSuccess, resolved, nil
}

// validationResultError contains field-level validation details.
type validationResultError struct {
	Errors []validationDetail
}

type validationDetail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *validationResultError) Error() string {
	if len(e.Errors) == 1 {
		return fmt.Sprintf("validation failed: %s: %s", e.Errors[0].Field, e.Errors[0].Message)
	}
	return fmt.Sprintf("validation failed: %d errors", len(e.Errors))
}

func collectValidationErrors(ve *jsonschema.ValidationError) []validationDetail {
	var details []validationDetail
	collectErrors(ve, &details)
	return details
}

func collectErrors(ve *jsonschema.ValidationError, details *[]validationDetail) {
	if len(ve.Causes) == 0 {
		field := "/" + strings.Join(ve.InstanceLocation, "/")
		if len(ve.InstanceLocation) == 0 {
			field = "/"
		}
		msg := ve.Error()
		if ve.ErrorKind != nil {
			printer := message.NewPrinter(language.English)
			msg = ve.ErrorKind.LocalizedString(printer)
		}
		*details = append(*details, validationDetail{
			Field:   field,
			Message: msg,
		})
		return
	}
	for _, cause := range ve.Causes {
		collectErrors(cause, details)
	}
}
