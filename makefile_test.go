package main_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestMakefileTargetsHaveHelp(t *testing.T) {
	content, err := os.ReadFile(filepath.Join(getRepoRoot(t), "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	target := regexp.MustCompile(`^([a-zA-Z0-9_.-]+):(?:[^=]|$)`)
	help := regexp.MustCompile(`##\s+\S`)
	for _, line := range strings.Split(string(content), "\n") {
		match := target.FindStringSubmatch(line)
		if match != nil && !strings.HasPrefix(match[1], ".") && !help.MatchString(line) {
			t.Errorf("make target %s has no help description", match[1])
		}
	}
}
