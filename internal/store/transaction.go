package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"stage-rigging-clearance/internal/application"
	"stage-rigging-clearance/internal/domain"
)

type transaction struct{ tx *sql.Tx }

func (s *Store) WithinTransaction(ctx context.Context, fn func(application.Transaction) error) error {
	tx, err := s.handle.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(&transaction{tx}); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
func (t *transaction) GetIdempotency(ctx context.Context, scope, key string) ([]byte, bool, error) {
	var raw []byte
	err := t.tx.QueryRowContext(ctx, `SELECT response FROM idempotency_records WHERE scope=? AND key=?`, scope, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	return raw, err == nil, err
}
func (t *transaction) PutIdempotency(ctx context.Context, scope, key string, response []byte) error {
	_, err := t.tx.ExecContext(ctx, `INSERT INTO idempotency_records(scope,key,response) VALUES(?,?,?)`, scope, key, response)
	if err != nil {
		return fmt.Errorf("保存幂等结果: %w", err)
	}
	return nil
}
func (t *transaction) LoadCase(ctx context.Context, id string) (*domain.RiggingCase, error) {
	return loadCase(ctx, t.tx, id)
}
func (t *transaction) CreateCase(ctx context.Context, c *domain.RiggingCase) error {
	_, err := t.tx.ExecContext(ctx, `INSERT INTO rigging_cases(id,show_name,venue_limit_kg,owner,scheduled_at,status,version,created_at) VALUES(?,?,?,?,?,?,?,?)`, c.ID, c.ShowName, c.VenueLimitKg, c.Owner, timeText(c.ScheduledAt), c.Status, c.Version, timeText(c.CreatedAt))
	return err
}
func (t *transaction) SaveCase(ctx context.Context, c *domain.RiggingCase, expected uint64) error {
	return saveAggregate(ctx, t.tx, c, expected)
}
func (t *transaction) AuditHead(ctx context.Context, id string) (string, uint64, error) {
	var digest string
	var seq uint64
	err := t.tx.QueryRowContext(ctx, `SELECT digest,sequence_no FROM audit_events WHERE case_id=? ORDER BY sequence_no DESC LIMIT 1`, id).Scan(&digest, &seq)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, nil
	}
	return digest, seq, err
}
func (t *transaction) AppendAudit(ctx context.Context, e domain.AuditEvent) error {
	_, err := t.tx.ExecContext(ctx, `INSERT INTO audit_events(case_id,sequence_no,kind,aggregate_version,payload,previous_digest,digest,occurred_at) VALUES(?,?,?,?,?,?,?,?)`, e.CaseID, e.Sequence, e.Kind, e.AggregateVersion, e.Payload, e.PreviousDigest, e.Digest, timeText(e.OccurredAt))
	return err
}
func (t *transaction) NextCertificateSerial(ctx context.Context) (uint64, error) {
	var n uint64
	err := t.tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(serial),0)+1 FROM release_certificates`).Scan(&n)
	return n, err
}
func (t *transaction) SaveCertificate(ctx context.Context, c *domain.ReleaseCertificate, snapshot []byte) error {
	_, err := t.tx.ExecContext(ctx, `INSERT INTO release_certificates(id,case_id,serial,frozen_version,snapshot_digest,audit_head_digest,reviewer,issued_at,snapshot) VALUES(?,?,?,?,?,?,?,?,?)`, c.ID, c.CaseID, c.Serial, c.FrozenVersion, c.SnapshotDigest, c.AuditHeadDigest, c.Reviewer, timeText(c.IssuedAt), snapshot)
	return err
}
