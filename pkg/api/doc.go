// Package api defines the stable public interfaces for Noda plugin authors.
//
// All plugins implement the Plugin interface and register nodes via NodeRegistration.
// Node logic is implemented via the NodeExecutor interface, which receives an
// ExecutionContext providing access to input data, auth, expressions, and logging.
//
// This package also defines standard error types, response types, and common
// service interfaces (storage, cache, connections) that plugins may depend on.
package api
