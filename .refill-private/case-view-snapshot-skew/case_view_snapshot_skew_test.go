package case_view_snapshot_skew_test

import (
	"context"
	"errors"
	"stage-rigging-clearance/internal/application"
	"stage-rigging-clearance/internal/domain"
	"testing"
	"time"
)

type stagedRepository struct {
	caseCaptured chan struct{}
	writeApplied chan struct{}
}

func newStagedRepository() *stagedRepository {
	return &stagedRepository{caseCaptured: make(chan struct{}), writeApplied: make(chan struct{})}
}

func (r *stagedRepository) ViewCase(context.Context, string) (*domain.RiggingCase, error) {
	close(r.caseCaptured)
	<-r.writeApplied
	return &domain.RiggingCase{ID: "case-edge", Version: 1, Status: domain.StatusDraft}, nil
}

func (r *stagedRepository) ListAudit(context.Context, string) ([]domain.AuditEvent, error) {
	<-r.caseCaptured
	close(r.writeApplied)
	return []domain.AuditEvent{{Sequence: 1, CaseID: "case-edge", AggregateVersion: 2, Kind: "LOAD_POINT_UPSERTED", Digest: "stable-digest"}}, nil
}

func (r *stagedRepository) WithinTransaction(context.Context, func(application.Transaction) error) error {
	return errors.New("unexpected transaction")
}
func (r *stagedRepository) ListCases(context.Context) ([]*domain.RiggingCase, error) {
	return nil, errors.New("unexpected list")
}
func (r *stagedRepository) LoadSnapshot(context.Context, string) ([]byte, *domain.ReleaseCertificate, error) {
	return nil, nil, errors.New("unexpected snapshot")
}
func (r *stagedRepository) FindValidationBatch(context.Context, string) (*domain.ValidationBatch, error) {
	return nil, errors.New("unexpected batch")
}
func (r *stagedRepository) FindCertificate(context.Context, *uint64, string) (*domain.ReleaseCertificate, []byte, error) {
	return nil, nil, errors.New("unexpected certificate")
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Unix(0, 0).UTC() }

type fixedIDs struct{}

func (fixedIDs) NewID(string) string { return "fixed-id" }

type fixedDigester struct{}

func (fixedDigester) EventDigest(string, string, uint64, string, []byte) string {
	return "stable-digest"
}
func (fixedDigester) SnapshotDigest([]byte) string { return "stable-digest" }

func TestCaseViewUsesOneConsistentSnapshot(t *testing.T) {
	repo := newStagedRepository()
	service := application.NewService(repo, fixedClock{}, fixedIDs{}, fixedDigester{})

	view, err := service.GetCase(context.Background(), "case-edge")
	if err != nil {
		t.Fatalf("GetCase 返回错误: %v", err)
	}
	if len(view.Timeline) == 0 {
		t.Fatal("GetCase 未返回审计时间线")
	}
	lastVersion := view.Timeline[len(view.Timeline)-1].AggregateVersion
	if view.Case.Version != lastVersion {
		t.Fatalf("同一方案视图必须来自一致快照: case version=%d, timeline version=%d", view.Case.Version, lastVersion)
	}
}
