package transport

import (
	"net/http"
	"stage-rigging-clearance/internal/application"
	"strconv"
)

func (s *Server) LoadPointHandler(w http.ResponseWriter, r *http.Request) {
	var in application.LoadPointInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	c, err := s.app.UpsertLoadPoint(r.Context(), r.PathValue("id"), in)
	respondCase(w, c, err)
}
func (s *Server) CueHandler(w http.ResponseWriter, r *http.Request) {
	var in application.CueInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	c, err := s.app.UpsertCue(r.Context(), r.PathValue("id"), in)
	respondCase(w, c, err)
}
func (s *Server) ChangeSetHandler(w http.ResponseWriter, r *http.Request) {
	var in application.ChangeSetInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	if len(in.LoadPoints)+len(in.Cues) > 250 {
		writeError(w, application.Invalid("TOO_MANY_ITEMS", "批量变更集最多包含 250 个项目"))
		return
	}
	c, err := s.app.ApplyChangeSet(r.Context(), r.PathValue("id"), in)
	respondCase(w, c, err)
}
func (s *Server) ValidateHandler(w http.ResponseWriter, r *http.Request) {
	var in application.WriteMeta
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	c, err := s.app.Validate(r.Context(), r.PathValue("id"), in)
	respondCase(w, c, err)
}
func (s *Server) StartRehearsalHandler(w http.ResponseWriter, r *http.Request) {
	var in application.StartRehearsalInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	c, err := s.app.StartRehearsal(r.Context(), r.PathValue("id"), in)
	respondCase(w, c, err)
}
func (s *Server) SaveCueResultHandler(w http.ResponseWriter, r *http.Request) {
	var in application.SaveCueResultInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	if in.RunID != "" && in.RunID != r.PathValue("runID") {
		writeError(w, application.Invalid("RUN_SCOPE_MISMATCH", "请求体 runId 与资源路径不一致"))
		return
	}
	if in.CueID != "" && in.CueID != r.PathValue("cueID") {
		writeError(w, application.Invalid("CUE_SCOPE_MISMATCH", "请求体 cueId 与资源路径不一致"))
		return
	}
	in.RunID, in.CueID = r.PathValue("runID"), r.PathValue("cueID")
	c, err := s.app.SaveCueResult(r.Context(), r.PathValue("id"), in)
	respondCase(w, c, err)
}
func (s *Server) CompleteRehearsalHandler(w http.ResponseWriter, r *http.Request) {
	var in application.CompleteRehearsalInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	c, err := s.app.CompleteRehearsal(r.Context(), r.PathValue("id"), in)
	respondCase(w, c, err)
}
func (s *Server) RemediationHandler(w http.ResponseWriter, r *http.Request) {
	var in application.RemediationInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	c, err := s.app.Remediate(r.Context(), r.PathValue("id"), in)
	respondCase(w, c, err)
}
func (s *Server) ReleaseHandler(w http.ResponseWriter, r *http.Request) {
	var in application.ReleaseInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	c, err := s.app.Release(r.Context(), r.PathValue("id"), in)
	respondCase(w, c, err)
}
func (s *Server) VerifyCertificateHandler(w http.ResponseWriter, r *http.Request) {
	ok, err := s.app.VerifyCertificate(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"valid": ok})
}

func (s *Server) IntegrityHandler(w http.ResponseWriter, r *http.Request) {
	from := 0
	if raw := r.URL.Query().Get("from"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, application.Invalid("INVALID_BREAKPOINT", "from 必须是整数"))
			return
		}
		from = value
	}
	report, err := s.app.VerifyIntegrity(r.Context(), r.PathValue("id"), from)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) ValidationBatchesHandler(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, application.Invalid("INVALID_RANGE", "limit 必须是整数"))
			return
		}
		limit = value
	}
	result, err := s.app.ListValidationBatches(r.Context(), r.PathValue("id"), limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) ValidationBatchDiffHandler(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.DiffValidationBatches(r.Context(), r.PathValue("id"), r.URL.Query().Get("fromBatchID"), r.URL.Query().Get("toBatchID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) CertificateLookupHandler(w http.ResponseWriter, r *http.Request) {
	var serial *uint64
	if raw := r.URL.Query().Get("serial"); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || value == 0 {
			writeError(w, application.Invalid("INVALID_SERIAL", "serial 必须是非零数字"))
			return
		}
		serial = &value
	}
	report, err := s.app.LookupCertificate(r.Context(), serial, r.URL.Query().Get("certificateID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
func respondCase(w http.ResponseWriter, c any, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, c)
}
