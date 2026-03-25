package cli

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/Z-Bra0/Ski/internal/testutil"
)

// multiSkillSpec keeps existing test call sites readable while reusing the
// shared fixture type from internal/testutil.
type multiSkillSpec = testutil.SkillSpec

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v\n%s", args, err, string(output))
	}
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v\n%s", args, err, strings.TrimSpace(string(output)))
	}
	return string(output)
}
