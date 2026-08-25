package transport

import "net/http"

func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
func (s *Server) WorkbenchHandler(w http.ResponseWriter, r *http.Request) {
	raw, err := webAssets.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "页面不可用", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(raw)
}
