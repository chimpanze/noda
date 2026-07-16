package api

// Cookie represents an HTTP cookie to set on the response.
// The json tags mirror the response.json node's config schema and the
// trace representation, so serialized responses use the same keys everywhere.
type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Path     string `json:"path"`
	Domain   string `json:"domain"`
	MaxAge   int    `json:"max_age"`
	Secure   bool   `json:"secure"`
	HTTPOnly bool   `json:"http_only"`
	SameSite string `json:"same_site"` // "Strict", "Lax", "None"
}

// HTTPResponse represents a structured HTTP response returned by workflow nodes.
type HTTPResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Cookies []Cookie          `json:"cookies"`
	Body    any               `json:"body"`
}
