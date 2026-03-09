// Package deps anchors project dependencies that are not yet imported in application code.
// This file ensures go mod tidy does not remove declared dependencies.
// Remove individual imports as they get used in actual application code.
package deps

import (
	_ "github.com/expr-lang/expr"
	_ "github.com/fsnotify/fsnotify"
	_ "github.com/getkin/kin-openapi/openapi3"
	_ "github.com/gofiber/contrib/jwt"
	_ "github.com/gofiber/fiber/v3"
	_ "github.com/golang-jwt/jwt/v5"
	_ "github.com/google/uuid"
	_ "github.com/h2non/bimg"
	_ "github.com/redis/go-redis/v9"
	_ "github.com/robfig/cron/v3"
	_ "github.com/santhosh-tekuri/jsonschema/v6"
	_ "github.com/spf13/afero"
	_ "github.com/spf13/cobra"
	_ "github.com/stretchr/testify/assert"
	_ "go.opentelemetry.io/otel"
	_ "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	_ "go.opentelemetry.io/otel/sdk"
	_ "gorm.io/driver/postgres"
	_ "gorm.io/gorm"
)
