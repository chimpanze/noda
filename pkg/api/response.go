package api

// Cookie represents an HTTP cookie to set on the response.
type Cookie struct {
	Name     string
	Value    string
	Path     string
	Domain   string
	MaxAge   int
	Secure   bool
	HTTPOnly bool
	SameSite string // "Strict", "Lax", "None"
}

// HTTPResponse represents a structured HTTP response returned by workflow nodes.
type HTTPResponse struct {
	Status  int
	Headers map[string]string
	Cookies []Cookie
	Body    any
}
