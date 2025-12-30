package srv

import (
	_ "embed"
	"net/http"
	"strings"
)

//go:embed swagger.json
var swaggerJSON []byte

// HandleAPIDocs serves the API documentation page using Scalar
func (s *Server) HandleAPIDocs(w http.ResponseWriter, r *http.Request) {
	// Check Accept header - if client wants JSON, serve the spec
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") && !strings.Contains(accept, "text/html") {
		w.Header().Set("Content-Type", "application/json")
		w.Write(swaggerJSON)
		return
	}

	// Serve Scalar UI
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(scalarHTML))
}

// HandleAPISpec serves the raw OpenAPI spec as JSON
func (s *Server) HandleAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(swaggerJSON)
}

const scalarHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>AoE4 Quote Database API</title>
    <link rel="icon" type="image/x-icon" href="/static/favicon.ico">
</head>
<body>
    <div id="app"></div>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
    <script>
        Scalar.createApiReference('#app', {
            url: '/api/openapi.json',
            proxyUrl: 'https://proxy.scalar.com',
            theme: 'purple',
            darkMode: true,
            hideDownloadButton: false,
            hiddenClients: [],
            defaultHttpClient: {
                targetKey: 'shell',
                clientKey: 'curl'
            }
        })
    </script>
</body>
</html>
`
