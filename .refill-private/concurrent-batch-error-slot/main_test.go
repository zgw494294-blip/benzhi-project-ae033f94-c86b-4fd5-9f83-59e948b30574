package concurrent_batch_error_slot_test

import (
	"context"
	"errors"
	"stage-rigging-clearance/internal/application"
	"stage-rigging-clearance/internal/domain"
	"testing"
)

var errBatchRead = errors.New("目标校验批次读取失败")

type coordinatedRepository struct {
	ready   chan string
	release chan struct{}
}

func (r *coordinatedRepository) WithinTransaction(context.Context, func(application.Transaction) error) error {
	return errors.New("unexpected transaction")
}

func (r *coordinatedRepository) ViewCase(context.Context, string) (*domain.RiggingCase, error) {
	return nil, errors.New("unexpected case lookup")
}

func (r *coordinatedRepository) ListCases(context.Context) ([]*domain.RiggingCase, error) {
	return nil, errors.New("unexpected case list")
}

func (r *coordinatedRepository) ListAudit(context.Context, string) ([]domain.AuditEvent, error) {
	return nil, errors.New("unexpected audit lookup")
}

func (r *coordinatedRepository) LoadSnapshot(context.Context, string) ([]byte, *domain.ReleaseCertificate, error) {
	return nil, nil, errors.New("unexpected snapshot lookup")
}

func (r *coordinatedRepository) FindValidationBatch(_ context.Context, id string) (*domain.ValidationBatch, error) {
	r.ready <- id
	<-r.release
	batch := &domain.ValidationBatch{ID: id, CaseID: "case-1", Issues: []domain.ValidationBatchIssue{}}
	if id == "batch-to" {
		return batch, errBatchRead
	}
	return batch, nil
}

func (r *coordinatedRepository) FindCertificate(context.Context, *uint64, string) (*domain.ReleaseCertificate, []byte, error) {
	return nil, nil, errors.New("unexpected certificate lookup")
}

func TestConcurrentBatchLookupPreservesErrors(t *testing.T) {
	repo := &coordinatedRepository{
		ready:   make(chan string, 2),
		release: make(chan struct{}),
	}
	service := application.NewService(repo, nil, nil, nil)
	result := make(chan error, 1)

	go func() {
		_, err := service.DiffValidationBatches(context.Background(), "case-1", "batch-from", "batch-to")
		result <- err
	}()

	seen := map[string]bool{<-repo.ready: true, <-repo.ready: true}
	if !seen["batch-from"] || !seen["batch-to"] {
		t.Fatalf("两个仓储读取未按预期进入受控并发点: %v", seen)
	}
	close(repo.release)

	if err := <-result; !errors.Is(err, errBatchRead) {
		t.Fatalf("并发读取丢失目标批次错误: got %v, want %v", err, errBatchRead)
	}
}
