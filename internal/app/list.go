package app

// SkillInfo holds display data for a single installed skill.
type SkillInfo struct {
	Name          string
	Enabled       bool
	Source        string
	UpstreamSkill string
	Commit        string
	Targets       []string
}

// List returns the skills declared in the active manifest, enriched with lock data.
func (s Service) List() ([]SkillInfo, error) {
	doc, lf, err := s.loadProjectState()
	if err != nil {
		return nil, err
	}

	infos := make([]SkillInfo, 0, len(doc.Skills))
	for _, skill := range doc.Skills {
		canonicalSource, upstreamSkill, err := canonicalSkillIdentity(skill.Source, skill.UpstreamSkill)
		if err != nil {
			return nil, err
		}
		info := SkillInfo{
			Name:          skill.Name,
			Enabled:       skillEnabled(skill),
			Source:        canonicalSource,
			UpstreamSkill: upstreamSkill,
			Targets:       effectiveTargetsForSkill(doc, skill),
		}
		if lock, ok := findLockSkill(lf.Skills, skill.Name); ok {
			if len(lock.Commit) >= 7 {
				info.Commit = lock.Commit[:7]
			} else {
				info.Commit = lock.Commit
			}
		}
		infos = append(infos, info)
	}

	return infos, nil
}
