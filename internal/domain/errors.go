package domain

import "fmt"

type Violation struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func invalidDetails(code, message string, details any) error {
	return Violation{Code: code, Message: message, Details: details}
}

func (v Violation) Error() string { return v.Code + ": " + v.Message }
func invalid(code, format string, args ...any) error {
	return Violation{Code: code, Message: fmt.Sprintf(format, args...)}
}
