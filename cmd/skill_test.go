package cmd

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/samzong/gmc/skills"
	kitup "github.com/samzong/kitup/go"
	kitupcobra "github.com/samzong/kitup/go-cobra"
	"github.com/stretchr/testify/require"
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

	require.NoError(t, cmd.Execute())
	require.FileExists(t, filepath.Join(home, ".agents", "skills", "gmc", "SKILL.md"))
}
