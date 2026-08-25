package transport

import (
	"net/http"
	"stage-rigging-clearance/internal/application"
)

func (s *Server) ListCasesHandler(w http.ResponseWriter, r *http.Request) {
	cases, err := s.app.ListCases(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": cases})
}
func (s *Server) CreateCaseHandler(w http.ResponseWriter, r *http.Request) {
	var in application.CreateCaseInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	c, err := s.app.CreateCase(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("ETag", versionETag(c.Version))
	writeJSON(w, http.StatusCreated, c)
}
func (s *Server) GetCaseHandler(w http.ResponseWriter, r *http.Request) {
	view, err := s.app.GetCase(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("ETag", versionETag(view.Case.Version))
	writeJSON(w, 200, view)
}
func versionETag(v uint64) string { return `"v` + uintString(v) + `"` }
func uintString(v uint64) string {
	if v == 0 {
		return "0"
	}
	buf := make([]byte, 0, 20)
	for v > 0 {
		buf = append(buf, byte('0'+v%10))
		v /= 10
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
