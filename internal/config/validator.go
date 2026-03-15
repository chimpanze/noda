package config

import (
	"encoding/json"
	"fmt"

	"github.com/chimpanze/noda/internal/config/schemas"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// ValidationError represents a single validation error with location context.
type ValidationError struct {
	FilePath   string
	JSONPath   string
	Message    string
	SchemaPath string
}

func (e *ValidationError) Error() string {
	if e.JSONPath != "" {
		return fmt.Sprintf("%s: %s: %s", e.FilePath, e.JSONPath, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.FilePath, e.Message)
}

// Validate validates all loaded config files against their JSON Schemas.
func Validate(rc *RawConfig) []ValidationError {
	var errs []ValidationError

	// Validate root config
	if rc.Root != nil {
		errs = append(errs, validateAgainstSchema("root.json", "noda.json", rc.Root)...)
	}

	// Validate routes
	for filePath, data := range rc.Routes {
		errs = append(errs, validateAgainstSchema("route.json", filePath, data)...)
	}

	// Validate workflows
	for filePath, data := range rc.Workflows {
		errs = append(errs, validateAgainstSchema("workflow.json", filePath, data)...)
	}

	// Validate workers
	for filePath, data := range rc.Workers {
		errs = append(errs, validateAgainstSchema("worker.json", filePath, data)...)
	}

	// Validate schedules
	for filePath, data := range rc.Schedules {
		errs = append(errs, validateAgainstSchema("schedule.json", filePath, data)...)
	}

	// Validate connections
	for filePath, data := range rc.Connections {
		errs = append(errs, validateAgainstSchema("connections.json", filePath, data)...)
	}

	// Validate tests
	for filePath, data := range rc.Tests {
		errs = append(errs, validateAgainstSchema("test.json", filePath, data)...)
	}

	// Validate models
	for filePath, data := range rc.Models {
		errs = append(errs, validateAgainstSchema("model.json", filePath, data)...)
	}

	return errs
}

func validateAgainstSchema(schemaName string, filePath string, data map[string]any) []ValidationError {
	schemaData, err := schemas.FS.ReadFile(schemaName)
	if err != nil {
		return []ValidationError{{FilePath: filePath, Message: fmt.Sprintf("internal error: schema %s not found", schemaName)}}
	}

	var schemaDoc any
	if err := json.Unmarshal(schemaData, &schemaDoc); err != nil {
		return []ValidationError{{FilePath: filePath, Message: fmt.Sprintf("internal error: invalid schema %s", schemaName)}}
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(schemaName, schemaDoc); err != nil {
		return []ValidationError{{FilePath: filePath, Message: fmt.Sprintf("internal error: could not add schema resource: %v", err)}}
	}

	schema, err := compiler.Compile(schemaName)
	if err != nil {
		return []ValidationError{{FilePath: filePath, Message: fmt.Sprintf("internal error: could not compile schema: %v", err)}}
	}

	validationErr := schema.Validate(data)
	if validationErr == nil {
		return nil
	}

	return extractValidationErrors(filePath, validationErr)
}

func extractValidationErrors(filePath string, err error) []ValidationError {
	valErr, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return []ValidationError{{FilePath: filePath, Message: err.Error()}}
	}

	var errs []ValidationError
	collectLeafErrors(filePath, valErr, &errs)
	return errs
}

func collectLeafErrors(filePath string, ve *jsonschema.ValidationError, errs *[]ValidationError) {
	if len(ve.Causes) == 0 {
		msg := ve.Error()
		if ve.ErrorKind != nil {
			printer := message.NewPrinter(language.English)
			msg = ve.ErrorKind.LocalizedString(printer)
		}
		instancePath := "/" + joinPath(ve.InstanceLocation)
		if len(ve.InstanceLocation) == 0 {
			instancePath = ""
		}
		schemaPath := ""
		if ve.ErrorKind != nil {
			schemaPath = "/" + joinPath(ve.ErrorKind.KeywordPath())
		}
		*errs = append(*errs, ValidationError{
			FilePath:   filePath,
			JSONPath:   instancePath,
			Message:    msg,
			SchemaPath: schemaPath,
		})
		return
	}

	for _, cause := range ve.Causes {
		collectLeafErrors(filePath, cause, errs)
	}
}

func joinPath(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "/"
		}
		result += p
	}
	return result
}
