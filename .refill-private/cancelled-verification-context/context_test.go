package cancelled_verification_context_test

import (
	"context"
	"errors"
	"testing"

	"stage-rigging-clearance/internal/application"
	"stage-rigging-clearance/internal/domain"
)

type cancellationRepository struct {
	cancel context.CancelFunc
}

func (r *cancellationRepository) WithinTransaction(context.Context, func(application.Transaction) error) error {
	return errors.New("unexpected transaction")
}

func (r *cancellationRepository) ViewCase(context.Context, string) (*domain.RiggingCase, error) {
	r.cancel()
	return &domain.RiggingCase{ID: "case-cancelled", Status: domain.StatusDraft}, nil
}

func (r *cancellationRepository) ListAudit(ctx context.Context, _ string) ([]domain.AuditEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []domain.AuditEvent{}, nil
}

func (r *cancellationRepository) ListCases(context.Context) ([]*domain.RiggingCase, error) {
	return nil, errors.New("unexpected list")
}

func (r *cancellationRepository) LoadSnapshot(context.Context, string) ([]byte, *domain.ReleaseCertificate, error) {
	return nil, nil, errors.New("unexpected snapshot")
}

func (r *cancellationRepository) FindValidationBatch(context.Context, string) (*domain.ValidationBatch, error) {
	return nil, errors.New("unexpected validation batch")
}

func (r *cancellationRepository) FindCertificate(context.Context, *uint64, string) (*domain.ReleaseCertificate, []byte, error) {
	return nil, nil, errors.New("unexpected certificate")
}

type deterministicDigester struct{}

func (deterministicDigester) EventDigest(string, string, uint64, string, []byte) string {
	return "digest"
}
func (deterministicDigester) SnapshotDigest([]byte) string { return "digest" }

func TestIntegrityVerificationPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	repo := &cancellationRepository{cancel: cancel}
	service := application.NewService(repo, nil, nil, deterministicDigester{})

	_, err := service.VerifyIntegrity(ctx, "case-cancelled", 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("TestIntegrityVerificationPropagatesCancellation: want context.Canceled after the first repository phase, got %v", err)
	}
}
