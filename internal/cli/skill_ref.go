package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Z-Bra0/Ski/internal/app"
)

func resolveSkillReferenceInfo(svc app.Service, raw string) (app.SkillInfo, bool, error) {
	index, ok, err := parseSkillReference(raw)
	if err != nil || !ok {
		return app.SkillInfo{}, ok, err
	}

	infos, err := svc.List()
	if err != nil {
		return app.SkillInfo{}, true, err
	}
	if index < 1 || index > len(infos) {
		return app.SkillInfo{}, true, fmt.Errorf("skill reference %q out of range (1-%d)", raw, len(infos))
	}
	return infos[index-1], true, nil
}

func resolveSkillReferenceName(svc app.Service, raw string) (string, bool, error) {
	info, ok, err := resolveSkillReferenceInfo(svc, raw)
	if err != nil || !ok {
		return "", ok, err
	}
	return info.Name, true, nil
}

func parseSkillReference(raw string) (int, bool, error) {
	if !strings.HasPrefix(raw, "@") {
		return 0, false, nil
	}
	index, err := strconv.Atoi(raw[1:])
	if err != nil || index <= 0 {
		return 0, true, fmt.Errorf("invalid skill reference %q", raw)
	}
	return index, true, nil
}
