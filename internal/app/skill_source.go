package app

import (
	"fmt"

	"github.com/Z-Bra0/Ski/internal/source"
)

type skillSourceRef struct {
	Base          source.Git
	UpstreamSkill string
}

func parseSkillSourceRef(rawSource, upstreamSkill string) (skillSourceRef, error) {
	src, err := source.ParseGit(rawSource)
	if err != nil {
		return skillSourceRef{}, err
	}

	selected, err := resolveUpstreamSkill(src.Skills, upstreamSkill)
	if err != nil {
		return skillSourceRef{}, err
	}

	return skillSourceRef{
		Base:          src.WithoutSkills(),
		UpstreamSkill: selected,
	}, nil
}

func resolveUpstreamSkill(selectors []string, upstreamSkill string) (string, error) {
	switch len(selectors) {
	case 0:
		return upstreamSkill, nil
	case 1:
		if upstreamSkill != "" && selectors[0] != upstreamSkill {
			return "", fmt.Errorf("source selector %q does not match upstream_skill %q", selectors[0], upstreamSkill)
		}
		if upstreamSkill != "" {
			return upstreamSkill, nil
		}
		return selectors[0], nil
	default:
		return "", fmt.Errorf("persisted source may select at most one skill")
	}
}

func canonicalSkillIdentity(rawSource, upstreamSkill string) (string, string, error) {
	ref, err := parseSkillSourceRef(rawSource, upstreamSkill)
	if err != nil {
		return "", "", err
	}
	return ref.Base.String(), ref.UpstreamSkill, nil
}

func sameSkillIdentity(leftSource, leftUpstream, rightSource, rightUpstream string) (bool, error) {
	leftCanonical, leftSkill, err := canonicalSkillIdentity(leftSource, leftUpstream)
	if err != nil {
		return false, err
	}
	rightCanonical, rightSkill, err := canonicalSkillIdentity(rightSource, rightUpstream)
	if err != nil {
		return false, err
	}
	return leftCanonical == rightCanonical && leftSkill == rightSkill, nil
}

func (s Service) loadSkillSourceForScope(rawSource, upstreamSkill string) (source.Git, error) {
	ref, err := parseSkillSourceRef(rawSource, upstreamSkill)
	if err != nil {
		return source.Git{}, err
	}

	src, err := s.loadSourceForScope(ref.Base.String())
	if err != nil {
		return source.Git{}, err
	}
	if ref.UpstreamSkill != "" {
		src = src.WithSkills([]string{ref.UpstreamSkill})
	}
	return src, nil
}
