package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"stage-rigging-clearance/internal/application"
	"stage-rigging-clearance/internal/domain"
	"time"
)

type sqlRunner interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func timeText(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
func optionalTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return timeText(*t)
}
func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("解析时间 %q 失败: %w", s, err)
	}
	return t, nil
}
func parseOptional(v sql.NullString) (*time.Time, error) {
	if !v.Valid {
		return nil, nil
	}
	t, err := parseTime(v.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
func loadCase(ctx context.Context, q sqlRunner, id string) (*domain.RiggingCase, error) {
	var c domain.RiggingCase
	var scheduled, created string
	var released sql.NullString
	err := q.QueryRowContext(ctx, `SELECT id,show_name,venue_limit_kg,owner,scheduled_at,status,version,created_at,released_at FROM rigging_cases WHERE id=?`, id).Scan(&c.ID, &c.ShowName, &c.VenueLimitKg, &c.Owner, &scheduled, &c.Status, &c.Version, &created, &released)
	if err == sql.ErrNoRows {
		return nil, application.NotFound("吊挂方案不存在")
	}
	if err != nil {
		return nil, err
	}
	c.ScheduledAt, err = parseTime(scheduled)
	if err != nil {
		return nil, fmt.Errorf("读取方案 scheduled_at: %w", err)
	}
	c.CreatedAt, err = parseTime(created)
	if err != nil {
		return nil, fmt.Errorf("读取方案 created_at: %w", err)
	}
	c.ReleasedAt, err = parseOptional(released)
	if err != nil {
		return nil, fmt.Errorf("读取方案 released_at: %w", err)
	}
	if err = loadPoints(ctx, q, &c); err != nil {
		return nil, err
	}
	if err = loadCues(ctx, q, &c); err != nil {
		return nil, err
	}
	if err = loadFindings(ctx, q, &c); err != nil {
		return nil, err
	}
	if err = loadRuns(ctx, q, &c); err != nil {
		return nil, err
	}
	if err = loadValidationBatches(ctx, q, &c); err != nil {
		return nil, err
	}
	if err = loadRemediationAttempts(ctx, q, &c); err != nil {
		return nil, err
	}
	cert, _, err := loadCertificate(ctx, q, id)
	if err != nil {
		return nil, err
	}
	c.Certificate = cert
	return &c, nil
}
func loadPoints(ctx context.Context, q sqlRunner, c *domain.RiggingCase) error {
	rows, err := q.QueryContext(ctx, `SELECT id,equipment_code,parent_point_id,rated_capacity_kg,static_load_kg,dynamic_factor_permille,revision FROM load_points WHERE case_id=? ORDER BY id`, c.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	c.LoadPoints = []domain.LoadPoint{}
	for rows.Next() {
		var p domain.LoadPoint
		var parent sql.NullString
		p.CaseID = c.ID
		if err := rows.Scan(&p.ID, &p.EquipmentCode, &parent, &p.RatedCapacityKg, &p.StaticLoadKg, &p.DynamicFactorPermille, &p.Revision); err != nil {
			return err
		}
		if parent.Valid {
			p.ParentPointID = &parent.String
		}
		c.LoadPoints = append(c.LoadPoints, p)
	}
	return rows.Err()
}
func loadCues(ctx context.Context, q sqlRunner, c *domain.RiggingCase) error {
	rows, err := q.QueryContext(ctx, `SELECT id,sequence_no,action,equipment_codes,mutex_group,expected_duration_ms,revision FROM scene_cues WHERE case_id=? ORDER BY sequence_no,id`, c.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	c.Cues = []domain.SceneCue{}
	for rows.Next() {
		var cue domain.SceneCue
		var codes string
		cue.CaseID = c.ID
		if err := rows.Scan(&cue.ID, &cue.Sequence, &cue.Action, &codes, &cue.MutexGroup, &cue.ExpectedDurationMs, &cue.Revision); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(codes), &cue.EquipmentCodes); err != nil {
			return err
		}
		c.Cues = append(c.Cues, cue)
	}
	return rows.Err()
}
func loadFindings(ctx context.Context, q sqlRunner, c *domain.RiggingCase) error {
	rows, err := q.QueryContext(ctx, `SELECT id,source,rule_code,severity,message,affected_refs,observed_revisions,status,remediation_revision,closed_at FROM safety_findings WHERE case_id=? ORDER BY id`, c.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	c.Findings = []domain.SafetyFinding{}
	for rows.Next() {
		var f domain.SafetyFinding
		var refs string
		var revisions string
		var rev sql.NullInt64
		var closed sql.NullString
		f.CaseID = c.ID
		if err := rows.Scan(&f.ID, &f.Source, &f.RuleCode, &f.Severity, &f.Message, &refs, &revisions, &f.Status, &rev, &closed); err != nil {
			return err
		}
		_ = json.Unmarshal([]byte(refs), &f.AffectedRefs)
		_ = json.Unmarshal([]byte(revisions), &f.ObservedRevisions)
		if rev.Valid {
			v := uint32(rev.Int64)
			f.RemediationRevision = &v
		}
		f.ClosedAt, err = parseOptional(closed)
		if err != nil {
			return fmt.Errorf("读取问题 closed_at: %w", err)
		}
		c.Findings = append(c.Findings, f)
	}
	return rows.Err()
}
func loadRuns(ctx context.Context, q sqlRunner, c *domain.RiggingCase) error {
	rows, err := q.QueryContext(ctx, `SELECT id,started_at,finished_at,cue_results,operator,outcome FROM rehearsal_runs WHERE case_id=? ORDER BY started_at,id`, c.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	c.Rehearsals = []domain.RehearsalRun{}
	for rows.Next() {
		var r domain.RehearsalRun
		var started, results string
		var finished sql.NullString
		r.CaseID = c.ID
		if err := rows.Scan(&r.ID, &started, &finished, &results, &r.Operator, &r.Outcome); err != nil {
			return err
		}
		r.StartedAt, err = parseTime(started)
		if err != nil {
			return fmt.Errorf("读取排练 started_at: %w", err)
		}
		r.FinishedAt, err = parseOptional(finished)
		if err != nil {
			return fmt.Errorf("读取排练 finished_at: %w", err)
		}
		_ = json.Unmarshal([]byte(results), &r.CueResults)
		c.Rehearsals = append(c.Rehearsals, r)
	}
	return rows.Err()
}

func loadValidationBatches(ctx context.Context, q sqlRunner, c *domain.RiggingCase) error {
	rows, err := q.QueryContext(ctx, `SELECT id,aggregate_version,started_at,completed_at,final_status,stale,changed_refs FROM validation_batches WHERE case_id=? ORDER BY completed_at,id`, c.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	c.ValidationBatches = []domain.ValidationBatch{}
	for rows.Next() {
		var b domain.ValidationBatch
		var started, completed, refs string
		var stale int
		b.CaseID = c.ID
		if err := rows.Scan(&b.ID, &b.AggregateVersion, &started, &completed, &b.FinalStatus, &stale, &refs); err != nil {
			return err
		}
		b.StartedAt, err = parseTime(started)
		if err != nil {
			return fmt.Errorf("读取校验批次 started_at: %w", err)
		}
		b.CompletedAt, err = parseTime(completed)
		if err != nil {
			return fmt.Errorf("读取校验批次 completed_at: %w", err)
		}
		b.Stale = stale != 0
		_ = json.Unmarshal([]byte(refs), &b.ChangedRefs)
		b.InputRevisions, b.Issues = map[string]uint32{}, []domain.ValidationBatchIssue{}
		c.ValidationBatches = append(c.ValidationBatches, b)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for i := range c.ValidationBatches {
		b := &c.ValidationBatches[i]
		inputRows, err := q.QueryContext(ctx, `SELECT ref,revision FROM validation_batch_inputs WHERE batch_id=? ORDER BY ref`, b.ID)
		if err != nil {
			return err
		}
		for inputRows.Next() {
			var ref string
			var revision uint32
			if err := inputRows.Scan(&ref, &revision); err != nil {
				inputRows.Close()
				return err
			}
			b.InputRevisions[ref] = revision
		}
		if err := inputRows.Err(); err != nil {
			inputRows.Close()
			return err
		}
		inputRows.Close()
		issueRows, err := q.QueryContext(ctx, `SELECT identity,rule_code,severity,message,affected_refs,state FROM validation_batch_issues WHERE batch_id=? ORDER BY state,identity`, b.ID)
		if err != nil {
			return err
		}
		for issueRows.Next() {
			var issue domain.ValidationBatchIssue
			var refs string
			if err := issueRows.Scan(&issue.Identity, &issue.RuleCode, &issue.Severity, &issue.Message, &refs, &issue.State); err != nil {
				issueRows.Close()
				return err
			}
			_ = json.Unmarshal([]byte(refs), &issue.AffectedRefs)
			b.Issues = append(b.Issues, issue)
		}
		if err := issueRows.Err(); err != nil {
			issueRows.Close()
			return err
		}
		issueRows.Close()
	}
	return nil
}

func loadRemediationAttempts(ctx context.Context, q sqlRunner, c *domain.RiggingCase) error {
	rows, err := q.QueryContext(ctx, `SELECT id,finding_id,observed_revisions,note,submitted_by,submitted_at,recheck_type,recheck_input,passed,conclusion FROM remediation_attempts WHERE case_id=? ORDER BY submitted_at,id`, c.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	c.RemediationAttempts = []domain.RemediationAttempt{}
	for rows.Next() {
		var a domain.RemediationAttempt
		var revisions, submitted string
		var input sql.NullString
		var passed int
		a.CaseID = c.ID
		if err := rows.Scan(&a.ID, &a.FindingID, &revisions, &a.Note, &a.SubmittedBy, &submitted, &a.RecheckType, &input, &passed, &a.Conclusion); err != nil {
			return err
		}
		a.SubmittedAt, err = parseTime(submitted)
		if err != nil {
			return fmt.Errorf("读取整改 submitted_at: %w", err)
		}
		a.Passed = passed != 0
		_ = json.Unmarshal([]byte(revisions), &a.ObservedRevisions)
		if input.Valid {
			var result domain.CueResult
			if json.Unmarshal([]byte(input.String), &result) == nil {
				a.RecheckInput = &result
			}
		}
		c.RemediationAttempts = append(c.RemediationAttempts, a)
	}
	return rows.Err()
}
func saveAggregate(ctx context.Context, tx *sql.Tx, c *domain.RiggingCase, expected uint64) error {
	res, err := tx.ExecContext(ctx, `UPDATE rigging_cases SET show_name=?,venue_limit_kg=?,owner=?,scheduled_at=?,status=?,version=?,released_at=? WHERE id=? AND version=?`, c.ShowName, c.VenueLimitKg, c.Owner, timeText(c.ScheduledAt), c.Status, c.Version, optionalTime(c.ReleasedAt), c.ID, expected)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return application.Conflict("VERSION_CONFLICT", "方案已被其他操作更新")
	}
	for _, table := range []string{"load_points", "scene_cues", "rehearsal_runs"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE case_id=?", c.ID); err != nil {
			return err
		}
	}
	for _, p := range c.LoadPoints {
		if _, err := tx.ExecContext(ctx, `INSERT INTO load_points VALUES(?,?,?,?,?,?,?,?)`, p.ID, c.ID, p.EquipmentCode, p.ParentPointID, p.RatedCapacityKg, p.StaticLoadKg, p.DynamicFactorPermille, p.Revision); err != nil {
			return err
		}
	}
	for _, q := range c.Cues {
		codes, _ := json.Marshal(q.EquipmentCodes)
		if _, err := tx.ExecContext(ctx, `INSERT INTO scene_cues VALUES(?,?,?,?,?,?,?,?)`, q.ID, c.ID, q.Sequence, q.Action, string(codes), q.MutexGroup, q.ExpectedDurationMs, q.Revision); err != nil {
			return err
		}
	}
	for _, f := range c.Findings {
		refs, _ := json.Marshal(f.AffectedRefs)
		revisions, _ := json.Marshal(f.ObservedRevisions)
		if _, err := tx.ExecContext(ctx, `INSERT INTO safety_findings VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET status=excluded.status,remediation_revision=excluded.remediation_revision,closed_at=excluded.closed_at`, f.ID, c.ID, f.Source, f.RuleCode, f.Severity, f.Message, string(refs), string(revisions), f.Status, f.RemediationRevision, optionalTime(f.ClosedAt)); err != nil {
			return err
		}
	}
	for _, r := range c.Rehearsals {
		results, _ := json.Marshal(r.CueResults)
		if _, err := tx.ExecContext(ctx, `INSERT INTO rehearsal_runs VALUES(?,?,?,?,?,?,?)`, r.ID, c.ID, timeText(r.StartedAt), optionalTime(r.FinishedAt), string(results), r.Operator, r.Outcome); err != nil {
			return err
		}
	}
	for _, b := range c.ValidationBatches {
		refs, _ := json.Marshal(b.ChangedRefs)
		if _, err := tx.ExecContext(ctx, `INSERT INTO validation_batches VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET stale=excluded.stale,changed_refs=excluded.changed_refs`, b.ID, c.ID, b.AggregateVersion, timeText(b.StartedAt), timeText(b.CompletedAt), b.FinalStatus, b.Stale, string(refs)); err != nil {
			return err
		}
		for ref, revision := range b.InputRevisions {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO validation_batch_inputs(batch_id,ref,revision) VALUES(?,?,?)`, b.ID, ref, revision); err != nil {
				return err
			}
		}
		for _, issue := range b.Issues {
			affected, _ := json.Marshal(issue.AffectedRefs)
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO validation_batch_issues(batch_id,identity,rule_code,severity,message,affected_refs,state) VALUES(?,?,?,?,?,?,?)`, b.ID, issue.Identity, issue.RuleCode, issue.Severity, issue.Message, string(affected), issue.State); err != nil {
				return err
			}
		}
	}
	for _, a := range c.RemediationAttempts {
		revisions, _ := json.Marshal(a.ObservedRevisions)
		var input any
		if a.RecheckInput != nil {
			raw, _ := json.Marshal(a.RecheckInput)
			input = string(raw)
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO remediation_attempts VALUES(?,?,?,?,?,?,?,?,?,?,?)`, a.ID, c.ID, a.FindingID, string(revisions), a.Note, a.SubmittedBy, timeText(a.SubmittedAt), a.RecheckType, input, a.Passed, a.Conclusion); err != nil {
			return err
		}
	}
	return nil
}
func loadCertificate(ctx context.Context, q sqlRunner, id string) (*domain.ReleaseCertificate, []byte, error) {
	var c domain.ReleaseCertificate
	var issued string
	var snapshot []byte
	err := q.QueryRowContext(ctx, `SELECT id,case_id,serial,frozen_version,snapshot_digest,audit_head_digest,reviewer,issued_at,snapshot FROM release_certificates WHERE case_id=?`, id).Scan(&c.ID, &c.CaseID, &c.Serial, &c.FrozenVersion, &c.SnapshotDigest, &c.AuditHeadDigest, &c.Reviewer, &issued, &snapshot)
	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("读取凭据: %w", err)
	}
	c.IssuedAt, err = parseTime(issued)
	if err != nil {
		return nil, nil, fmt.Errorf("读取凭据 issued_at: %w", err)
	}
	return &c, snapshot, nil
}
