package domain

import "sort"

func (c *RiggingCase) UpsertLoadPoint(p LoadPoint) error {
	issues := c.ApplyChangeSet(ChangeSet{LoadPoints: []LoadPoint{p}})
	if len(issues) > 0 {
		return invalid(issues[0].Code, "%s", issues[0].Message)
	}
	return nil
}
func (c *RiggingCase) checkConnections() error {
	for _, p := range c.LoadPoints {
		if p.ParentPointID != nil && c.PointByID(*p.ParentPointID) == nil {
			return invalid("UNKNOWN_PARENT", "父吊点 %s 不存在", *p.ParentPointID)
		}
	}
	for _, p := range c.LoadPoints {
		seen := map[string]bool{}
		q := &p
		for q.ParentPointID != nil {
			if seen[q.ID] {
				return invalid("CONNECTION_CYCLE", "吊点连接形成环")
			}
			seen[q.ID] = true
			q = c.PointByID(*q.ParentPointID)
			if q == nil {
				break
			}
		}
	}
	return nil
}
func (c *RiggingCase) UpsertCue(cue SceneCue) error {
	issues := c.ApplyChangeSet(ChangeSet{Cues: []SceneCue{cue}})
	if len(issues) > 0 {
		return invalid(issues[0].Code, "%s", issues[0].Message)
	}
	return nil
}

func (c *RiggingCase) sortCues() {
	sort.Slice(c.Cues, func(i, j int) bool {
		if c.Cues[i].Sequence == c.Cues[j].Sequence {
			return c.Cues[i].ID < c.Cues[j].ID
		}
		return c.Cues[i].Sequence < c.Cues[j].Sequence
	})
}
