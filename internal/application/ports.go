package application

import (
	"context"
	"stage-rigging-clearance/internal/domain"
	"time"
)

type Clock interface{ Now() time.Time }
type IDGenerator interface{ NewID(prefix string) string }
type Digester interface {
	EventDigest(previous string, caseID string, version uint64, kind string, payload []byte) string
	SnapshotDigest(snapshot []byte) string
}
type Transaction interface {
	LoadCase(context.Context, string) (*domain.RiggingCase, error)
	SaveCase(context.Context, *domain.RiggingCase, uint64) error
	CreateCase(context.Context, *domain.RiggingCase) error
	GetIdempotency(context.Context, string, string) ([]byte, bool, error)
	PutIdempotency(context.Context, string, string, []byte) error
	AuditHead(context.Context, string) (string, uint64, error)
	AppendAudit(context.Context, domain.AuditEvent) error
	NextCertificateSerial(context.Context) (uint64, error)
	SaveCertificate(context.Context, *domain.ReleaseCertificate, []byte) error
}
type Repository interface {
	WithinTransaction(context.Context, func(Transaction) error) error
	ViewCase(context.Context, string) (*domain.RiggingCase, error)
	ListCases(context.Context) ([]*domain.RiggingCase, error)
	ListAudit(context.Context, string) ([]domain.AuditEvent, error)
	LoadSnapshot(context.Context, string) ([]byte, *domain.ReleaseCertificate, error)
	FindValidationBatch(context.Context, string) (*domain.ValidationBatch, error)
	FindCertificate(context.Context, *uint64, string) (*domain.ReleaseCertificate, []byte, error)
}

type AppError struct {
	Kind    string `json:"kind"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func (e *AppError) Error() string    { return e.Message }
func Invalid(code, msg string) error { return &AppError{Kind: "validation", Code: code, Message: msg} }
func InvalidDetails(code, msg string, details any) error {
	return &AppError{Kind: "validation", Code: code, Message: msg, Details: details}
}
func Conflict(code, msg string) error { return &AppError{Kind: "conflict", Code: code, Message: msg} }
func NotFound(msg string) error       { return &AppError{Kind: "not_found", Code: "NOT_FOUND", Message: msg} }
