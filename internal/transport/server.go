package transport

import (
	"embed"
	"io/fs"
	"net/http"
	"stage-rigging-clearance/internal/application"
)

//go:embed web/*
var webAssets embed.FS

type Server struct {
	app *application.Service
	mux *http.ServeMux
}

func NewServer(app *application.Service) *Server {
	s := &Server{app: app, mux: http.NewServeMux()}
	s.routes()
	return s
}
func (s *Server) Handler() http.Handler { return securityHeaders(s.mux) }
func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.HealthHandler)
	s.mux.HandleFunc("GET /workbench", s.WorkbenchHandler)
	static, _ := fs.Sub(webAssets, "web")
	s.mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(static))))
	s.mux.HandleFunc("GET /api/v1/rigging-cases", s.ListCasesHandler)
	s.mux.HandleFunc("POST /api/v1/rigging-cases", s.CreateCaseHandler)
	s.mux.HandleFunc("GET /api/v1/rigging-cases/{id}", s.GetCaseHandler)
	s.mux.HandleFunc("PUT /api/v1/rigging-cases/{id}/load-points", s.LoadPointHandler)
	s.mux.HandleFunc("PUT /api/v1/rigging-cases/{id}/cues", s.CueHandler)
	s.mux.HandleFunc("POST /api/v1/rigging-cases/{id}/change-sets", s.ChangeSetHandler)
	s.mux.HandleFunc("POST /api/v1/rigging-cases/{id}/validate", s.ValidateHandler)
	s.mux.HandleFunc("GET /api/v1/rigging-cases/{id}/validation-batches", s.ValidationBatchesHandler)
	s.mux.HandleFunc("GET /api/v1/rigging-cases/{id}/validation-batches/diff", s.ValidationBatchDiffHandler)
	s.mux.HandleFunc("POST /api/v1/rigging-cases/{id}/rehearsals", s.StartRehearsalHandler)
	s.mux.HandleFunc("PUT /api/v1/rigging-cases/{id}/rehearsals/{runID}/cue-results/{cueID}", s.SaveCueResultHandler)
	s.mux.HandleFunc("POST /api/v1/rigging-cases/{id}/rehearsals/complete", s.CompleteRehearsalHandler)
	s.mux.HandleFunc("POST /api/v1/rigging-cases/{id}/remediations", s.RemediationHandler)
	s.mux.HandleFunc("POST /api/v1/rigging-cases/{id}/release", s.ReleaseHandler)
	s.mux.HandleFunc("GET /api/v1/rigging-cases/{id}/certificate/verify", s.VerifyCertificateHandler)
	s.mux.HandleFunc("GET /api/v1/rigging-cases/{id}/integrity", s.IntegrityHandler)
	s.mux.HandleFunc("GET /api/v1/release-certificates", s.CertificateLookupHandler)
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
