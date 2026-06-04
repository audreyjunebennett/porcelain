package operatorapi

// ErrorBody is a simple {"error": "..."} response used by several /api/ui routes.
type ErrorBody struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"`
}

// OKResponse is {"ok": true} (and optional extra fields via embedding at call sites).
type OKResponse struct {
	OK bool `json:"ok"`
}
