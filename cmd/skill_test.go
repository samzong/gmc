package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/samzong/gmc/skills"
	kitup "github.com/samzong/kitup/go"
	kitupcobra "github.com/samzong/kitup/go-cobra"
)

func TestSkillCommandInstallsBundledSkill(t *testing.T) {
	home := t.TempDir()
	var out bytes.Buffer
	cmd := kitupcobra.NewSkillCommand(kitupcobra.Options{
		AppID:  "gmc",
		Bundle: kitup.FSBundle(skills.GMC, "gmc"),
		Home:   home,
		Out:    &out,
	})
	cmd.SetArgs([]string{"install", "--agent", "codex", "--yes"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "gmc", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}
