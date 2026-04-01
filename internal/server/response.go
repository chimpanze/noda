package server

import (
	"github.com/chimpanze/noda/pkg/api"
	"github.com/gofiber/fiber/v3"
)

// writeHTTPResponse writes an api.HTTPResponse to a Fiber context.
func writeHTTPResponse(c fiber.Ctx, resp *api.HTTPResponse) error {
	// Set headers
	for k, v := range resp.Headers {
		c.Set(k, v)
	}

	// Set cookies
	for _, cookie := range resp.Cookies {
		fc := &fiber.Cookie{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Path:     cookie.Path,
			Domain:   cookie.Domain,
			MaxAge:   cookie.MaxAge,
			Secure:   cookie.Secure,
			HTTPOnly: cookie.HTTPOnly,
			SameSite: cookie.SameSite,
		}
		c.Cookie(fc)
	}

	// Set status and body
	c.Status(resp.Status)

	if resp.Body == nil {
		return c.Send(nil)
	}

	// Binary body (e.g. from response.file) — send raw bytes.
	if b, ok := resp.Body.([]byte); ok {
		return c.Send(b)
	}

	return c.JSON(resp.Body)
}

// writeErrorResponse writes a standardized error response.
func writeErrorResponse(c fiber.Ctx, status int, resp ErrorResponse) error {
	return c.Status(status).JSON(resp)
}
