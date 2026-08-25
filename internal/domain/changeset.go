package domain

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const MaxChangeSetItems = 250
const maxLoadKg int64 = 1_000_000_000_000
const maxCueDurationMs int64 = 60 * 60 * 1000

type ChangeSet struct {
	LoadPoints []LoadPoint `json:"loadPoints"`
	Cues       []SceneCue  `json:"cues"`
}

type ChangeIssue struct {
	ItemType   string `json:"itemType"`
	Index      int    `json:"index"`
	BusinessID string `json:"businessId,omitempty"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (c *RiggingCase) ApplyChangeSet(changes ChangeSet) []ChangeIssue {
	if err := c.EnsureMutable(); err != nil {
		return []ChangeIssue{{ItemType: "changeSet", Index: -1, Code: "CASE_FROZEN", Message: err.Error()}}
	}
	if err := c.RequireStatus(StatusDraft, StatusRemediating); err != nil {
		return []ChangeIssue{{ItemType: "changeSet", Index: -1, Code: "INVALID_STATE", Message: err.Error()}}
	}
	issues := preflightChangeSet(c, changes)
	if len(issues) > 0 {
		return issues
	}
	changedRefs := []string{}
	for _, incoming := range changes.LoadPoints {
		incoming.ID = strings.TrimSpace(incoming.ID)
		incoming.EquipmentCode = strings.TrimSpace(incoming.EquipmentCode)
		if incoming.ParentPointID != nil {
			parent := strings.TrimSpace(*incoming.ParentPointID)
			incoming.ParentPointID = &parent
		}
		incoming.CaseID = c.ID
		if current := c.PointByID(incoming.ID); current != nil {
			incoming.Revision = current.Revision
			if !samePoint(*current, incoming) {
				incoming.Revision++
				*current = incoming
				changedRefs = append(changedRefs, incoming.ID)
			}
		} else {
			incoming.Revision = 1
			c.LoadPoints = append(c.LoadPoints, incoming)
			changedRefs = append(changedRefs, incoming.ID)
		}
	}
	for _, incoming := range changes.Cues {
		incoming.ID = strings.TrimSpace(incoming.ID)
		incoming.Action = strings.TrimSpace(incoming.Action)
		incoming.CaseID = c.ID
		incoming.EquipmentCodes = normalizedRefs(incoming.EquipmentCodes)
		if current := c.CueByID(incoming.ID); current != nil {
			incoming.Revision = current.Revision
			if !sameCue(*current, incoming) {
				incoming.Revision++
				*current = incoming
				changedRefs = append(changedRefs, incoming.ID)
			}
		} else {
			incoming.Revision = 1
			c.Cues = append(c.Cues, incoming)
			changedRefs = append(changedRefs, incoming.ID)
		}
	}
	if len(changedRefs) > 0 {
		sort.Slice(c.LoadPoints, func(i, j int) bool { return c.LoadPoints[i].ID < c.LoadPoints[j].ID })
		c.sortCues()
		c.markValidationStale(changedRefs...)
		c.Bump()
	}
	return nil
}

func preflightChangeSet(c *RiggingCase, changes ChangeSet) []ChangeIssue {
	if len(changes.LoadPoints)+len(changes.Cues) == 0 {
		return []ChangeIssue{{ItemType: "changeSet", Index: -1, Code: "EMPTY_CHANGE_SET", Message: "批量变更集不能为空"}}
	}
	if len(changes.LoadPoints)+len(changes.Cues) > MaxChangeSetItems {
		return []ChangeIssue{{ItemType: "changeSet", Index: -1, Code: "TOO_MANY_ITEMS", Message: fmt.Sprintf("批量变更集最多包含 %d 个项目", MaxChangeSetItems)}}
	}
	points := make(map[string]LoadPoint, len(c.LoadPoints)+len(changes.LoadPoints))
	pointLocations := map[string]int{}
	for _, p := range c.LoadPoints {
		points[p.ID] = p
	}
	issues := []ChangeIssue{}
	for i, p := range changes.LoadPoints {
		id := strings.TrimSpace(p.ID)
		p.ID, p.EquipmentCode = id, strings.TrimSpace(p.EquipmentCode)
		if p.ParentPointID != nil {
			parent := strings.TrimSpace(*p.ParentPointID)
			p.ParentPointID = &parent
		}
		add := func(code, message string) { issues = append(issues, ChangeIssue{"loadPoint", i, id, code, message}) }
		if id == "" {
			add("INVALID_LOAD_POINT", "吊点编号不能为空")
		}
		if strings.TrimSpace(p.EquipmentCode) == "" {
			add("INVALID_LOAD_POINT", "设备编号不能为空")
		}
		if p.RatedCapacityKg <= 0 || p.RatedCapacityKg > maxLoadKg || p.StaticLoadKg < 0 || p.StaticLoadKg > maxLoadKg {
			add("INVALID_LOAD_VALUE", "额定容量或静载越界")
		}
		if p.DynamicFactorPermille < 1000 || p.DynamicFactorPermille > 3000 {
			add("INVALID_DYNAMIC_FACTOR", "动态系数必须在 1000 至 3000 千分比之间")
		}
		if previous, exists := pointLocations[id]; id != "" && exists {
			add("DUPLICATE_LOAD_POINT", fmt.Sprintf("吊点编号与项目 %d 重复", previous))
		} else if id != "" {
			pointLocations[id] = i
		}
		points[id] = p
	}
	equipmentOwner := map[string]string{}
	finalPointIDs := make([]string, 0, len(points))
	for id := range points {
		finalPointIDs = append(finalPointIDs, id)
	}
	sort.Strings(finalPointIDs)
	for _, pointID := range finalPointIDs {
		p := points[pointID]
		code := strings.TrimSpace(p.EquipmentCode)
		if prior, exists := equipmentOwner[code]; code != "" && exists && prior != p.ID {
			target, other := p.ID, prior
			idx, changed := pointLocations[target]
			if !changed {
				target, other, idx = prior, p.ID, pointLocations[prior]
			}
			issues = append(issues, ChangeIssue{"loadPoint", idx, target, "DUPLICATE_EQUIPMENT", fmt.Sprintf("设备编号 %s 与吊点 %s 重复", code, other)})
		} else if code != "" {
			equipmentOwner[code] = p.ID
		}
	}
	for _, id := range finalPointIDs {
		p := points[id]
		if p.ParentPointID != nil {
			parent := strings.TrimSpace(*p.ParentPointID)
			if _, ok := points[parent]; !ok || parent == "" {
				issues = append(issues, ChangeIssue{"loadPoint", pointLocations[id], id, "UNKNOWN_PARENT", fmt.Sprintf("父吊点 %s 不存在", parent)})
			} else if parent == id {
				issues = append(issues, ChangeIssue{"loadPoint", pointLocations[id], id, "CONNECTION_CYCLE", "吊点不能连接自身"})
			}
		}
	}
	cycleIDs := connectionCycleIDs(points)
	for _, id := range cycleIDs {
		issues = append(issues, ChangeIssue{"loadPoint", pointLocations[id], id, "CONNECTION_CYCLE", "吊点连接形成环"})
	}
	cues := make(map[string]SceneCue, len(c.Cues)+len(changes.Cues))
	cueLocations := map[string]int{}
	for _, cue := range c.Cues {
		cues[cue.ID] = cue
	}
	for i, cue := range changes.Cues {
		id := strings.TrimSpace(cue.ID)
		cue.ID, cue.Action, cue.EquipmentCodes = id, strings.TrimSpace(cue.Action), normalizedRefs(cue.EquipmentCodes)
		add := func(code, message string) { issues = append(issues, ChangeIssue{"cue", i, id, code, message}) }
		if id == "" {
			add("INVALID_CUE", "提示编号不能为空")
		}
		if cue.Sequence <= 0 {
			add("INVALID_CUE_SEQUENCE", "提示顺序必须大于零")
		}
		if strings.TrimSpace(cue.Action) == "" {
			add("INVALID_CUE", "提示动作不能为空")
		}
		if cue.ExpectedDurationMs <= 0 || cue.ExpectedDurationMs > maxCueDurationMs {
			add("INVALID_DURATION", "提示时长必须大于零且不超过 60 分钟")
		}
		if len(cue.EquipmentCodes) == 0 {
			add("INVALID_CUE", "提示至少关联一台设备")
		}
		for _, code := range cue.EquipmentCodes {
			if _, ok := equipmentOwner[strings.TrimSpace(code)]; !ok {
				add("UNKNOWN_EQUIPMENT", fmt.Sprintf("设备 %s 不存在", code))
			}
		}
		if previous, exists := cueLocations[id]; id != "" && exists {
			add("DUPLICATE_CUE", fmt.Sprintf("提示编号与项目 %d 重复", previous))
		} else if id != "" {
			cueLocations[id] = i
		}
		cues[id] = cue
	}
	sequenceOwner := map[int]string{}
	finalCueIDs := make([]string, 0, len(cues))
	for id := range cues {
		finalCueIDs = append(finalCueIDs, id)
	}
	sort.Strings(finalCueIDs)
	for _, cueID := range finalCueIDs {
		cue := cues[cueID]
		if prior, exists := sequenceOwner[cue.Sequence]; cue.Sequence > 0 && exists && prior != cue.ID {
			target, other := cue.ID, prior
			idx, changed := cueLocations[target]
			if !changed {
				target, other, idx = prior, cue.ID, cueLocations[prior]
			}
			issues = append(issues, ChangeIssue{"cue", idx, target, "DUPLICATE_SEQUENCE", fmt.Sprintf("提示顺序 %d 与提示 %s 重复", cue.Sequence, other)})
		} else if cue.Sequence > 0 {
			sequenceOwner[cue.Sequence] = cue.ID
		}
	}
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].ItemType != issues[j].ItemType {
			return issues[i].ItemType < issues[j].ItemType
		}
		if issues[i].Index != issues[j].Index {
			return issues[i].Index < issues[j].Index
		}
		if issues[i].BusinessID != issues[j].BusinessID {
			return issues[i].BusinessID < issues[j].BusinessID
		}
		return issues[i].Code < issues[j].Code
	})
	return deduplicateChangeIssues(issues)
}

func connectionCycleIDs(points map[string]LoadPoint) []string {
	bad := map[string]bool{}
	for id := range points {
		path, at := []string{}, map[string]int{}
		for current := id; current != ""; {
			if start, ok := at[current]; ok {
				for _, item := range path[start:] {
					bad[item] = true
				}
				break
			}
			at[current], path = len(path), append(path, current)
			p, ok := points[current]
			if !ok || p.ParentPointID == nil {
				break
			}
			current = *p.ParentPointID
		}
	}
	out := make([]string, 0, len(bad))
	for id := range bad {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func deduplicateChangeIssues(in []ChangeIssue) []ChangeIssue {
	out := make([]ChangeIssue, 0, len(in))
	seen := map[string]bool{}
	for _, issue := range in {
		key := fmt.Sprintf("%s:%d:%s:%s", issue.ItemType, issue.Index, issue.BusinessID, issue.Code)
		if !seen[key] {
			seen[key] = true
			out = append(out, issue)
		}
	}
	return out
}

func samePoint(a, b LoadPoint) bool {
	a.CaseID, b.CaseID, a.Revision, b.Revision = "", "", 0, 0
	return reflect.DeepEqual(a, b)
}

func sameCue(a, b SceneCue) bool {
	a.CaseID, b.CaseID, a.Revision, b.Revision = "", "", 0, 0
	a.EquipmentCodes, b.EquipmentCodes = normalizedRefs(a.EquipmentCodes), normalizedRefs(b.EquipmentCodes)
	return reflect.DeepEqual(a, b)
}
