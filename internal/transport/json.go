package transport

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"stage-rigging-clearance/internal/application"
	"strings"
)

const maxBody = 1 << 20

type errorEnvelope struct {
	Error apiError `json:"error"`
}
type apiError struct {
	Kind    string `json:"kind"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		return application.Invalid("CONTENT_TYPE_REQUIRED", "Content-Type 必须是 application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return application.Invalid("INVALID_JSON", "请求 JSON 无效: "+err.Error())
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return application.Invalid("INVALID_JSON", "请求体只能包含一个 JSON 对象")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	out := apiError{Kind: "internal", Code: "INTERNAL_ERROR", Message: "服务内部错误"}
	var app *application.AppError
	if errors.As(err, &app) {
		out = apiError{Kind: app.Kind, Code: app.Code, Message: app.Message, Details: app.Details}
		switch app.Kind {
		case "validation":
			status = http.StatusUnprocessableEntity
		case "conflict":
			status = http.StatusConflict
		case "not_found":
			status = http.StatusNotFound
		}
	}
	writeJSON(w, status, errorEnvelope{out})
}
