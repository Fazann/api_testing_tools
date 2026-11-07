package backend

// Endpoint is one route of the remote backend. Paths may hold {name}
// placeholders that are filled from Request.PathParams, e.g. "/collections/{id}".
type Endpoint struct {
	Method string
	Path   string
}

// String returns "METHOD /path", for errors.
func (e Endpoint) String() string { return e.Method + " " + e.Path }

// The backend's routes. Add new ones here so every call site uses the same
// method and path. The backend is not built yet: only the health check exists
// so far, and its path is a placeholder until the server defines it.
var (
	EndpointHealth = Endpoint{Method: "GET", Path: "/health"}
)
