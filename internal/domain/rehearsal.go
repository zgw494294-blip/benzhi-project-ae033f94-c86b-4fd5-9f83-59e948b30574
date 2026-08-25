package domain

import (
	"sort"
	"strings"
	"time"
)

func (c *RiggingCase) StartRehearsal(id, operator string, now time.Time) (*RehearsalRun, error) {
	if err := c.RequireStatus(StatusValidated); err != nil {
		return nil, err
	}
	if len(c.ValidationBatches) == 0 {
		return nil, invalid("VALIDATION_BASELINE_REQUIRED", "缺少可追溯的校验批次")
	}
	latest := c.ValidationBatches[len(c.ValidationBatches)-1]
	if latest.Stale {
		return nil, invalidDetails("VALIDATION_STALE", "最近校验批次已过期，不能开始排练", map[string]any{"batchId": latest.ID, "changedRefs": latest.ChangedRefs})
	}
	if strings.TrimSpace(operator) == "" {
		return nil, invalid("INVALID_OPERATOR", "排练操作员不能为空")
	}
	if len(c.Cues) > 200 {
		return nil, invalid("REHEARSAL_TOO_LARGE", "单次排练最多包含 200 条提示")
	}
	var totalDuration int64
	for _, cue := range c.Cues {
		if cue.ExpectedDurationMs > 60*60*1000-totalDuration {
			return nil, invalid("REHEARSAL_TOO_LONG", "单次排练预计时长不能超过 60 分钟")
		}
		totalDuration += cue.ExpectedDurationMs
	}
	r := RehearsalRun{ID: id, CaseID: c.ID, StartedAt: now.UTC(), Operator: strings.TrimSpace(operator), Outcome: OutcomePending, CueResults: []CueResult{}}
	c.Rehearsals = append(c.Rehearsals, r)
	c.Status = StatusRehearsing
	c.Bump()
	return &c.Rehearsals[len(c.Rehearsals)-1], nil
}

func (c *RiggingCase) SaveCueResult(runID string, result CueResult, operator string, now time.Time) error {
	if err := c.RequireStatus(StatusRehearsing); err != nil {
		return err
	}
	run := c.rehearsalByID(runID)
	if run == nil {
		return invalid("RUN_NOT_FOUND", "排练记录不存在")
	}
	if run.FinishedAt != nil || run.Outcome != OutcomePending {
		return invalid("RUN_COMPLETED", "已完成排练不可修改")
	}
	if c.CueByID(result.CueID) == nil {
		return invalid("CUE_SCOPE_MISMATCH", "提示不属于当前方案")
	}
	if result.PeakKg < 0 {
		return invalid("INVALID_PEAK", "实测峰值不能为负数")
	}
	if strings.TrimSpace(operator) == "" {
		return invalid("INVALID_OPERATOR", "记录操作员不能为空")
	}
	if (!result.Success || strings.TrimSpace(result.Deviation) != "") && strings.TrimSpace(result.Evidence) == "" {
		return invalid("EVIDENCE_REQUIRED", "失败或偏差结果必须提供证据")
	}
	result.Operator, result.UpdatedAt = strings.TrimSpace(operator), now.UTC()
	for i := range run.CueResults {
		if run.CueResults[i].CueID == result.CueID {
			run.CueResults[i] = result
			sortCueResults(run.CueResults, c)
			c.Bump()
			return nil
		}
	}
	run.CueResults = append(run.CueResults, result)
	sortCueResults(run.CueResults, c)
	c.Bump()
	return nil
}

func (c *RiggingCase) CompleteRehearsal(runID string, legacyResults []CueResult, now time.Time, newID func() string) error {
	if err := c.RequireStatus(StatusRehearsing); err != nil {
		return err
	}
	run := c.rehearsalByID(runID)
	if run == nil {
		return invalid("RUN_NOT_FOUND", "排练记录不存在")
	}
	if run.FinishedAt != nil || run.Outcome != OutcomePending {
		return invalid("RUN_COMPLETED", "排练已经完成")
	}
	if len(legacyResults) > 0 {
		seen := map[string]bool{}
		for _, result := range legacyResults {
			if c.CueByID(result.CueID) == nil || seen[result.CueID] {
				return invalid("INVALID_CUE_RESULT", "提示结果重复或未知")
			}
			seen[result.CueID] = true
			if result.PeakKg < 0 {
				return invalid("INVALID_PEAK", "实测峰值不能为负数")
			}
			if (!result.Success || strings.TrimSpace(result.Deviation) != "") && strings.TrimSpace(result.Evidence) == "" {
				return invalid("EVIDENCE_REQUIRED", "失败或偏差结果必须提供证据")
			}
			result.Operator, result.UpdatedAt = run.Operator, now.UTC()
			run.CueResults = append(run.CueResults, result)
		}
		sortCueResults(run.CueResults, c)
	}
	stored := map[string]CueResult{}
	for _, result := range run.CueResults {
		if _, duplicate := stored[result.CueID]; duplicate {
			return invalid("DUPLICATE_CUE_RESULT", "同一提示存在重复结果")
		}
		stored[result.CueID] = result
	}
	missing := []string{}
	for _, cue := range c.Cues {
		if _, ok := stored[cue.ID]; !ok {
			missing = append(missing, cue.ID)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return invalidDetails("INCOMPLETE_REHEARSAL", "仍有提示尚未登记", map[string]any{"missingCueIds": missing})
	}
	blocked := false
	for _, cue := range c.Cues {
		result := stored[cue.ID]
		peakRefs, peakMessage := c.peakViolation(result)
		if len(peakRefs) > 0 {
			blocked = true
			observed := map[string]uint32{result.CueID: c.RevisionOf(result.CueID)}
			for _, ref := range peakRefs {
				observed[ref] = c.RevisionOf(ref)
			}
			c.Findings = append(c.Findings, SafetyFinding{ID: newID(), CaseID: c.ID, Source: FindingRehearsal, RuleCode: "REHEARSAL_PEAK", Severity: SeverityBlocking, Message: peakMessage, AffectedRefs: append([]string{result.CueID}, peakRefs...), ObservedRevisions: observed, Status: FindingOpen})
		}
		if !result.Success || strings.TrimSpace(result.Deviation) != "" {
			blocked = true
			c.Findings = append(c.Findings, SafetyFinding{ID: newID(), CaseID: c.ID, Source: FindingRehearsal, RuleCode: "REHEARSAL_DEVIATION", Severity: SeverityBlocking, Message: "排练提示存在阻断偏差: " + result.Deviation, AffectedRefs: []string{result.CueID}, ObservedRevisions: map[string]uint32{result.CueID: c.RevisionOf(result.CueID)}, Status: FindingOpen})
		}
	}
	finished := now.UTC()
	run.FinishedAt = &finished
	if blocked {
		run.Outcome, c.Status = OutcomeBlocked, StatusRemediating
	} else {
		run.Outcome, c.Status = OutcomePassed, StatusReadyForReview
	}
	c.Bump()
	return nil
}

func (c *RiggingCase) rehearsalByID(id string) *RehearsalRun {
	for i := range c.Rehearsals {
		if c.Rehearsals[i].ID == id {
			return &c.Rehearsals[i]
		}
	}
	return nil
}

func sortCueResults(results []CueResult, c *RiggingCase) {
	sequence := map[string]int{}
	for _, cue := range c.Cues {
		sequence[cue.ID] = cue.Sequence
	}
	sort.Slice(results, func(i, j int) bool {
		if sequence[results[i].CueID] == sequence[results[j].CueID] {
			return results[i].CueID < results[j].CueID
		}
		return sequence[results[i].CueID] < sequence[results[j].CueID]
	})
}

func (c *RiggingCase) peakViolation(result CueResult) ([]string, string) {
	cue := c.CueByID(result.CueID)
	if cue == nil || result.PeakKg == 0 {
		return nil, ""
	}
	refs := []string{}
	for _, equipment := range cue.EquipmentCodes {
		for _, point := range c.LoadPoints {
			if point.EquipmentCode == equipment && result.PeakKg > point.RatedCapacityKg {
				refs = append(refs, point.ID)
			}
		}
	}
	refs = normalizedRefs(refs)
	if len(refs) == 0 {
		return nil, ""
	}
	return refs, "排练实测峰值超过关联吊点额定容量"
}
