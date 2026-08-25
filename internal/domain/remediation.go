package domain

import (
	"strings"
	"time"
)

type RemediationRequest struct {
	FindingID    string
	Revision     uint32
	Note         string
	SubmittedBy  string
	RecheckInput *CueResult
}

func (c *RiggingCase) AttemptRemediation(id string, request RemediationRequest, now time.Time) (*RemediationAttempt, error) {
	if err := c.RequireStatus(StatusRemediating); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.Note) == "" || strings.TrimSpace(request.SubmittedBy) == "" {
		return nil, invalid("INVALID_REMEDIATION", "整改说明和提交人不能为空")
	}
	var target *SafetyFinding
	for i := range c.Findings {
		if c.Findings[i].ID == request.FindingID {
			target = &c.Findings[i]
			break
		}
	}
	if target == nil || target.Status != FindingOpen || target.Severity != SeverityBlocking {
		return nil, invalid("FINDING_NOT_OPEN", "原阻断问题不存在或已关闭")
	}
	current := map[string]uint32{}
	for _, ref := range target.AffectedRefs {
		current[ref] = c.RevisionOf(ref)
	}
	attempt := RemediationAttempt{ID: id, CaseID: c.ID, FindingID: target.ID, ObservedRevisions: current, Note: strings.TrimSpace(request.Note), SubmittedBy: strings.TrimSpace(request.SubmittedBy), SubmittedAt: now.UTC(), RecheckType: target.Source, RecheckInput: request.RecheckInput}
	if target.Source == FindingValidation {
		changed := false
		for _, ref := range target.AffectedRefs {
			if c.RevisionOf(ref) > target.ObservedRevisions[ref] {
				changed = true
				break
			}
		}
		if !changed {
			attempt.Conclusion = "受影响输入 revision 未高于原问题基线"
		} else {
			hit := false
			for _, result := range Evaluate(c) {
				if result.Code == target.RuleCode && refsIntersect(result.Refs, target.AffectedRefs) {
					hit = true
					break
				}
			}
			if hit {
				attempt.Conclusion = "目标规则复验仍然命中"
			} else {
				attempt.Passed, attempt.Conclusion = true, "受影响确定性规则复验通过"
			}
		}
	} else {
		if request.RecheckInput == nil {
			attempt.Conclusion = "现场问题必须提交提示重测数据"
		} else if !containsRef(target.AffectedRefs, request.RecheckInput.CueID) {
			attempt.Conclusion = "重测提示不属于原问题影响范围"
		} else if strings.TrimSpace(request.RecheckInput.Evidence) == "" {
			attempt.Conclusion = "提示重测证据不能为空"
		} else if !request.RecheckInput.Success || strings.TrimSpace(request.RecheckInput.Deviation) != "" {
			attempt.Conclusion = "提示重测仍为失败或存在偏差"
		} else {
			refs, _ := c.peakViolation(*request.RecheckInput)
			if len(refs) > 0 {
				attempt.Conclusion = "提示重测峰值仍超过关联吊点额定容量"
			} else {
				attempt.Passed, attempt.Conclusion = true, "提示重测成功且峰值合格"
			}
		}
	}
	c.RemediationAttempts = append(c.RemediationAttempts, attempt)
	if attempt.Passed {
		closed := now.UTC()
		target.Status, target.ClosedAt = FindingClosed, &closed
		if request.Revision > 0 {
			revision := request.Revision
			target.RemediationRevision = &revision
		}
		if len(c.OpenBlockingFindings()) == 0 {
			c.Status = StatusReadyForReview
		}
	}
	c.Bump()
	return &c.RemediationAttempts[len(c.RemediationAttempts)-1], nil
}

func refsIntersect(a, b []string) bool {
	for _, ref := range a {
		if containsRef(b, ref) {
			return true
		}
	}
	return false
}
func containsRef(refs []string, target string) bool {
	for _, ref := range refs {
		if ref == target {
			return true
		}
	}
	return false
}

func (c *RiggingCase) Remediate(findingID string, revision uint32, note string, now time.Time) error {
	if err := c.RequireStatus(StatusRemediating); err != nil {
		return err
	}
	if revision == 0 || note == "" {
		return invalid("INVALID_REMEDIATION", "整改修订号和说明不能为空")
	}
	var target *SafetyFinding
	for i := range c.Findings {
		if c.Findings[i].ID == findingID {
			target = &c.Findings[i]
			break
		}
	}
	if target == nil || target.Status != FindingOpen || target.Severity != SeverityBlocking {
		return invalid("FINDING_NOT_OPEN", "原阻断问题不存在或已关闭")
	}
	traceable := false
	for _, ref := range target.AffectedRefs {
		observed := target.ObservedRevisions[ref]
		if p := c.PointByID(ref); p != nil && p.Revision >= revision && p.Revision > observed {
			traceable = true
		}
		if q := c.CueByID(ref); q != nil && q.Revision >= revision && q.Revision > observed {
			traceable = true
		}
	}
	if target.RuleCode == "REHEARSAL_DEVIATION" {
		for _, ref := range target.AffectedRefs {
			if q := c.CueByID(ref); q != nil && q.Revision >= revision && q.Revision > target.ObservedRevisions[ref] {
				traceable = true
			}
		}
	}
	if !traceable {
		return invalid("UNTRACEABLE_REMEDIATION", "整改未关联到受影响输入的有效修订")
	}
	for _, r := range Evaluate(c) {
		if r.Code == target.RuleCode {
			return invalid("RECHECK_FAILED", "受影响规则复验仍未通过")
		}
	}
	target.RemediationRevision = &revision
	closed := now.UTC()
	target.ClosedAt = &closed
	target.Status = FindingClosed
	if len(c.OpenBlockingFindings()) == 0 {
		c.Status = StatusReadyForReview
	}
	c.Bump()
	return nil
}
func (c *RiggingCase) Release(reviewer string, now time.Time) error {
	if err := c.EnsureReadyForRelease(); err != nil {
		return err
	}
	if reviewer == "" {
		return invalid("INVALID_REVIEWER", "安全复核员不能为空")
	}
	if len(c.OpenBlockingFindings()) > 0 {
		return invalid("OPEN_FINDINGS", "仍有未关闭阻断问题")
	}
	c.Status = StatusReleased
	t := now.UTC()
	c.ReleasedAt = &t
	c.Bump()
	return nil
}
