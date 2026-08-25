package store

import (
	"context"
	"database/sql"
	"errors"
	"stage-rigging-clearance/internal/application"
	"stage-rigging-clearance/internal/domain"
)

func (s *Store) ViewCase(ctx context.Context, id string) (*domain.RiggingCase, error) {
	return loadCase(ctx, s.db, id)
}
func (s *Store) ListCases(ctx context.Context) ([]*domain.RiggingCase, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM rigging_cases ORDER BY created_at DESC,id`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	out := make([]*domain.RiggingCase, 0, len(ids))
	for _, id := range ids {
		c, err := s.ViewCase(context.WithoutCancel(ctx), id)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}
func (s *Store) ListAudit(ctx context.Context, id string) ([]domain.AuditEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT sequence_no,kind,aggregate_version,payload,previous_digest,digest,occurred_at FROM audit_events WHERE case_id=? ORDER BY sequence_no`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AuditEvent{}
	for rows.Next() {
		var e domain.AuditEvent
		var at string
		e.CaseID = id
		if err := rows.Scan(&e.Sequence, &e.Kind, &e.AggregateVersion, &e.Payload, &e.PreviousDigest, &e.Digest, &at); err != nil {
			return nil, err
		}
		e.OccurredAt = parseTime(at)
		out = append(out, e)
	}
	return out, rows.Err()
}
func (s *Store) LoadSnapshot(ctx context.Context, id string) ([]byte, *domain.ReleaseCertificate, error) {
	cert, snapshot, err := loadCertificate(ctx, s.db, id)
	return snapshot, cert, err
}

func (s *Store) FindValidationBatch(ctx context.Context, id string) (*domain.ValidationBatch, error) {
	var caseID string
	err := s.db.QueryRowContext(ctx, `SELECT case_id FROM validation_batches WHERE id=?`, id).Scan(&caseID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, application.NotFound("校验批次不存在")
	}
	if err != nil {
		return nil, err
	}
	c, err := s.ViewCase(ctx, caseID)
	if err != nil {
		return nil, err
	}
	for i := range c.ValidationBatches {
		if c.ValidationBatches[i].ID == id {
			b := c.ValidationBatches[i]
			return &b, nil
		}
	}
	return nil, application.NotFound("校验批次不存在")
}

func (s *Store) FindCertificate(ctx context.Context, serial *uint64, certificateID string) (*domain.ReleaseCertificate, []byte, error) {
	query := `SELECT case_id FROM release_certificates WHERE id=?`
	arg := any(certificateID)
	if serial != nil {
		query, arg = `SELECT case_id FROM release_certificates WHERE serial=?`, *serial
	}
	var caseID string
	err := s.db.QueryRowContext(ctx, query, arg).Scan(&caseID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, application.NotFound("放行凭据不存在")
	}
	if err != nil {
		return nil, nil, err
	}
	cert, snapshot, err := loadCertificate(ctx, s.db, caseID)
	return cert, snapshot, err
}
