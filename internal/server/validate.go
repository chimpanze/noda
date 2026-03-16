package server

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// bodyValidator validates request bodies against a pre-compiled JSON Schema.
type bodyValidator struct {
	compiled   *jsonschema.Schema
	compileErr error
}

// newBodyValidator compiles the given JSON Schema map for reuse across requests.
func newBodyValidator(schema map[string]any) *bodyValidator {
	if schema == nil {
		return &bodyValidator{compileErr: fmt.Errorf("nil schema")}
	}

	c := jsonschema.NewCompiler()
	c.AssertFormat()

	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return &bodyValidator{compileErr: fmt.Errorf("marshal schema: %w", err)}
	}

	var schemaDoc any
	if err := json.Unmarshal(schemaBytes, &schemaDoc); err != nil {
		return &bodyValidator{compileErr: fmt.Errorf("unmarshal schema: %w", err)}
	}

	if err := c.AddResource("schema.json", schemaDoc); err != nil {
		return &bodyValidator{compileErr: fmt.Errorf("add schema resource: %w", err)}
	}

	compiled, err := c.Compile("schema.json")
	if err != nil {
		return &bodyValidator{compileErr: fmt.Errorf("compile schema: %w", err)}
	}

	return &bodyValidator{compiled: compiled}
}

// Validate checks body against the compiled schema.
// Returns nil if valid, or a bodyValidationError with field-level details.
func (v *bodyValidator) Validate(body any) error {
	if v.compiled == nil {
		return fmt.Errorf("body validation: %w", v.compileErr)
	}

	// Round-trip through JSON for proper type handling
	dataBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("body validation: marshal body: %w", err)
	}

	var dataDoc any
	if err := json.Unmarshal(dataBytes, &dataDoc); err != nil {
		return fmt.Errorf("body validation: unmarshal body: %w", err)
	}

	err = v.compiled.Validate(dataDoc)
	if err != nil {
		ve, ok := err.(*jsonschema.ValidationError)
		if ok {
			errors := collectBodyValidationErrors(ve)
			return &bodyValidationError{Errors: errors}
		}
		return fmt.Errorf("body validation: %w", err)
	}

	return nil
}

// bodyValidationError contains field-level validation details for request body validation.
type bodyValidationError struct {
	Errors []bodyValidationDetail
}

// bodyValidationDetail holds a single field-level validation error.
type bodyValidationDetail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *bodyValidationError) Error() string {
	if len(e.Errors) == 1 {
		return fmt.Sprintf("body validation failed: %s: %s", e.Errors[0].Field, e.Errors[0].Message)
	}
	return fmt.Sprintf("body validation failed: %d errors", len(e.Errors))
}

func collectBodyValidationErrors(ve *jsonschema.ValidationError) []bodyValidationDetail {
	var details []bodyValidationDetail
	collectBodyErrors(ve, &details)
	return details
}

// responseValidator validates response bodies against per-status-code JSON Schemas.
type responseValidator struct {
	schemas map[int]*bodyValidator
	mode    string // "", "warn", "strict"
}

// newResponseValidator builds a responseValidator from the route's response config.
// It iterates keys of responseCfg, parsing numeric keys as status codes and compiling
// each nested "schema" via newBodyValidator. Non-numeric keys are skipped.
func newResponseValidator(responseCfg map[string]any, mode string) *responseValidator {
	rv := &responseValidator{
		schemas: make(map[int]*bodyValidator),
		mode:    mode,
	}
	for key, val := range responseCfg {
		status, err := strconv.Atoi(key)
		if err != nil {
			continue // skip non-numeric keys like "validate", "description"
		}
		def, ok := val.(map[string]any)
		if !ok {
			continue
		}
		schema, ok := def["schema"].(map[string]any)
		if !ok {
			continue
		}
		rv.schemas[status] = newBodyValidator(schema)
	}
	return rv
}

// ValidateResponse checks the response body against the schema for the given status code.
// Returns nil if no schema is defined for that status or if validation passes.
func (rv *responseValidator) ValidateResponse(status int, body any) error {
	v, ok := rv.schemas[status]
	if !ok {
		return nil
	}
	return v.Validate(body)
}

func collectBodyErrors(ve *jsonschema.ValidationError, details *[]bodyValidationDetail) {
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
		*details = append(*details, bodyValidationDetail{
			Field:   field,
			Message: msg,
		})
		return
	}
	for _, cause := range ve.Causes {
		collectBodyErrors(cause, details)
	}
}
