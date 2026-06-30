package cmd

import (
	"os"

	"github.com/samzong/gmc/skills"
	kitup "github.com/samzong/kitup/go"
	kitupcobra "github.com/samzong/kitup/go-cobra"
)

func init() {
	skillCmd := kitupcobra.NewSkillCommand(kitupcobra.Options{
		AppID:        "gmc",
		Bundle:       kitup.FSBundle(skills.GMC, "gmc"),
		DefaultScope: kitup.UserScope,
		StdinTTY:     stdinTTY(),
	})
	skillCmd.GroupID = "other"
	rootCmd.AddCommand(skillCmd)
}

func stdinTTY() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
