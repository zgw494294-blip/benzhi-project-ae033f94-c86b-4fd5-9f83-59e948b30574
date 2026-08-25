package domain

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type RuleResult struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Refs     []string `json:"affectedRefs"`
}

func Evaluate(c *RiggingCase) []RuleResult {
	out := []RuleResult{}
	points := make(map[string]LoadPoint, len(c.LoadPoints))
	ids := make([]string, 0, len(c.LoadPoints))
	for _, p := range c.LoadPoints {
		points[p.ID], ids = p, append(ids, p.ID)
	}
	sort.Strings(ids)
	children := map[string][]string{}
	structurallyInvalid := map[string]bool{}
	for _, id := range ids {
		p := points[id]
		if p.ParentPointID != nil {
			if _, ok := points[*p.ParentPointID]; !ok {
				out = append(out, RuleResult{"ORPHAN_PARENT", SeverityBlocking, fmt.Sprintf("吊点 %s 的父吊点 %s 不存在", id, *p.ParentPointID), []string{id, *p.ParentPointID}})
				structurallyInvalid[id] = true
			} else {
				children[*p.ParentPointID] = append(children[*p.ParentPointID], id)
			}
		}
	}
	for parent := range children {
		sort.Strings(children[parent])
	}
	for _, id := range connectionCycleIDs(points) {
		structurallyInvalid[id] = true
		out = append(out, RuleResult{"CONNECTION_CYCLE", SeverityBlocking, fmt.Sprintf("吊点 %s 无法归属连接树：连接形成环", id), []string{id}})
	}
	own := map[string]int64{}
	for _, id := range ids {
		p := points[id]
		dynamic, ok := dynamicLoad(p.StaticLoadKg, p.DynamicFactorPermille)
		if !ok {
			structurallyInvalid[id] = true
			out = append(out, RuleResult{"CALCULATION_OVERFLOW", SeverityBlocking, fmt.Sprintf("吊点 %s 自有动态载荷计算越界", id), []string{id}})
			continue
		}
		own[id] = dynamic
		if dynamic > p.RatedCapacityKg {
			out = append(out, RuleResult{"POINT_OWN_CAPACITY", SeverityBlocking, fmt.Sprintf("吊点 %s 自有动载 %dkg 超过额定 %dkg", id, dynamic, p.RatedCapacityKg), []string{id}})
		} else if lowMargin(dynamic, p.RatedCapacityKg) {
			out = append(out, RuleResult{"POINT_OWN_MARGIN", SeverityWarning, fmt.Sprintf("吊点 %s 自有动载 %dkg，额定余量 %dkg 低于 15%%", id, dynamic, p.RatedCapacityKg-dynamic), []string{id}})
		}
	}
	totals := map[string]int64{}
	contributors := map[string][]string{}
	visiting := map[string]bool{}
	var cascade func(string) (int64, []string, bool)
	cascade = func(id string) (int64, []string, bool) {
		if refs, ok := contributors[id]; ok {
			return totals[id], refs, true
		}
		if visiting[id] || structurallyInvalid[id] {
			return 0, nil, false
		}
		value, ok := own[id]
		if !ok {
			return 0, nil, false
		}
		visiting[id] = true
		refs := []string{id}
		for _, child := range children[id] {
			childTotal, childRefs, childOK := cascade(child)
			if !childOK || childTotal > math.MaxInt64-value {
				visiting[id] = false
				return 0, nil, false
			}
			value += childTotal
			refs = append(refs, childRefs...)
		}
		visiting[id] = false
		refs = normalizedRefs(refs)
		totals[id], contributors[id] = value, refs
		return value, refs, true
	}
	roots := []string{}
	for _, id := range ids {
		if points[id].ParentPointID == nil {
			roots = append(roots, id)
		}
	}
	for _, id := range ids {
		total, refs, ok := cascade(id)
		if !ok {
			if !structurallyInvalid[id] {
				out = append(out, RuleResult{"CALCULATION_OVERFLOW", SeverityBlocking, fmt.Sprintf("吊点 %s 级联动态载荷累计越界或后代不可计算", id), []string{id}})
			}
			continue
		}
		p := points[id]
		if total > p.RatedCapacityKg {
			out = append(out, RuleResult{"POINT_CAPACITY", SeverityBlocking, fmt.Sprintf("吊点 %s 自有动载 %dkg、后代贡献 %dkg、级联总量 %dkg，超过额定 %dkg", id, own[id], total-own[id], total, p.RatedCapacityKg), refs})
		} else if lowMargin(total, p.RatedCapacityKg) {
			out = append(out, RuleResult{"POINT_MARGIN", SeverityWarning, fmt.Sprintf("吊点 %s 级联总量 %dkg，额定余量 %dkg 低于 15%%", id, total, p.RatedCapacityKg-total), refs})
		}
	}
	var venueTotal int64
	venueRefs := []string{}
	venueOK := true
	for _, root := range roots {
		total, refs, ok := cascade(root)
		if !ok || total > math.MaxInt64-venueTotal {
			venueOK = false
			break
		}
		venueTotal += total
		venueRefs = append(venueRefs, refs...)
	}
	if !venueOK {
		out = append(out, RuleResult{"CALCULATION_OVERFLOW", SeverityBlocking, "场地净载累计越界，不能形成有效校验结论", []string{"venue"}})
	} else if venueTotal > c.VenueLimitKg {
		out = append(out, RuleResult{"VENUE_CAPACITY", SeverityBlocking, fmt.Sprintf("连接树根节点净总量 %dkg 超过场地限制 %dkg", venueTotal, c.VenueLimitKg), append([]string{"venue"}, normalizedRefs(venueRefs)...)})
	}
	for _, id := range ids {
		if points[id].ParentPointID == nil {
			continue
		}
		rootFound := false
		seen := map[string]bool{}
		current := id
		for current != "" && !seen[current] {
			seen[current] = true
			p, ok := points[current]
			if !ok {
				break
			}
			if p.ParentPointID == nil {
				rootFound = true
				break
			}
			current = *p.ParentPointID
		}
		if !rootFound {
			out = append(out, RuleResult{"UNROOTED_POINT", SeverityBlocking, fmt.Sprintf("吊点 %s 无法归属连接树根节点", id), []string{id}})
		}
	}
	for i := 0; i < len(c.Cues); i++ {
		for j := i + 1; j < len(c.Cues); j++ {
			a, b := c.Cues[i], c.Cues[j]
			if a.MutexGroup != "" && a.MutexGroup == b.MutexGroup && a.Sequence == b.Sequence {
				out = append(out, RuleResult{"MUTEX_ACTION", SeverityBlocking, fmt.Sprintf("提示 %s 与 %s 在互斥组 %s 同时执行", a.ID, b.ID, a.MutexGroup), []string{a.ID, b.ID}})
			}
		}
	}
	if len(c.LoadPoints) == 0 {
		out = append(out, RuleResult{"LOAD_REQUIRED", SeverityBlocking, "至少需要一个吊点", []string{"loadPoints"}})
	}
	if len(c.Cues) == 0 {
		out = append(out, RuleResult{"CUE_REQUIRED", SeverityBlocking, "至少需要一个换景提示", []string{"cues"}})
	}
	for i := range out {
		out[i].Refs = normalizedRefs(out[i].Refs)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Code == out[j].Code {
			return issueIdentity(out[i].Code, out[i].Refs) < issueIdentity(out[j].Code, out[j].Refs)
		}
		return out[i].Code < out[j].Code
	})
	return deduplicateRuleResults(out)
}

func dynamicLoad(static, factor int64) (int64, bool) {
	if static < 0 || factor < 0 || (factor != 0 && static > (math.MaxInt64-999)/factor) {
		return 0, false
	}
	return (static*factor + 999) / 1000, true
}

func lowMargin(load, capacity int64) bool {
	if capacity <= 0 || load < 0 {
		return false
	}
	if load > capacity {
		return false
	}
	threshold := (capacity/100)*15 + (capacity%100)*15/100
	return capacity-load < threshold
}

func normalizedRefs(refs []string) []string {
	set := map[string]bool{}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref != "" {
			set[ref] = true
		}
	}
	out := make([]string, 0, len(set))
	for ref := range set {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

func issueIdentity(code string, refs []string) string {
	return code + "|" + strings.Join(normalizedRefs(refs), ",")
}

func deduplicateRuleResults(in []RuleResult) []RuleResult {
	out := make([]RuleResult, 0, len(in))
	seen := map[string]bool{}
	for _, result := range in {
		key := issueIdentity(result.Code, result.Refs)
		if !seen[key] {
			seen[key] = true
			out = append(out, result)
		}
	}
	return out
}

func (c *RiggingCase) RunValidation(batchID string, started, completed time.Time, newFindingID func() string) ValidationBatch {
	results := Evaluate(c)
	previous := map[string]ValidationBatchIssue{}
	if len(c.ValidationBatches) > 0 {
		for _, issue := range c.ValidationBatches[len(c.ValidationBatches)-1].Issues {
			if issue.State != IssueResolved {
				previous[issue.Identity] = issue
			}
		}
	}
	current := map[string]bool{}
	issues := make([]ValidationBatchIssue, 0, len(results)+len(previous))
	for _, result := range results {
		identity := issueIdentity(result.Code, result.Refs)
		current[identity] = true
		state := IssueNew
		if _, ok := previous[identity]; ok {
			state = IssuePersisting
		}
		issues = append(issues, ValidationBatchIssue{Identity: identity, RuleCode: result.Code, Severity: result.Severity, Message: result.Message, AffectedRefs: result.Refs, State: state})
	}
	for identity, old := range previous {
		if !current[identity] {
			old.State = IssueResolved
			issues = append(issues, old)
		}
	}
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].State != issues[j].State {
			return issues[i].State < issues[j].State
		}
		return issues[i].Identity < issues[j].Identity
	})
	for i := range c.Findings {
		if c.Findings[i].Source == FindingValidation && c.Findings[i].Status == FindingOpen {
			closed := completed.UTC()
			c.Findings[i].Status = FindingClosed
			c.Findings[i].ClosedAt = &closed
		}
	}
	blocking := false
	for _, result := range results {
		observed := map[string]uint32{}
		for _, ref := range result.Refs {
			observed[ref] = c.RevisionOf(ref)
		}
		c.Findings = append(c.Findings, SafetyFinding{ID: newFindingID(), CaseID: c.ID, Source: FindingValidation, RuleCode: result.Code, Severity: result.Severity, Message: result.Message, AffectedRefs: result.Refs, ObservedRevisions: observed, Status: FindingOpen})
		if result.Severity == SeverityBlocking {
			blocking = true
		}
	}
	if blocking {
		c.Status = StatusDraft
	} else {
		c.Status = StatusValidated
	}
	baseline := map[string]uint32{}
	for _, p := range c.LoadPoints {
		baseline[p.ID] = p.Revision
	}
	for _, cue := range c.Cues {
		baseline[cue.ID] = cue.Revision
	}
	batch := ValidationBatch{ID: batchID, CaseID: c.ID, AggregateVersion: c.Version, InputRevisions: baseline, StartedAt: started.UTC(), CompletedAt: completed.UTC(), FinalStatus: c.Status, Issues: issues}
	c.ValidationBatches = append(c.ValidationBatches, batch)
	c.Bump()
	return batch
}

func (c *RiggingCase) Validate(newID func() string) []SafetyFinding {
	now := time.Now().UTC()
	c.RunValidation(newID(), now, now, newID)
	return c.Findings
}
