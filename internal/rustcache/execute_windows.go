package rustcache

import (
	"os"
	"os/exec"
)

func execute(args, env []string) error {
	command := exec.Command(args[0], args[1:]...)
	command.Env = env
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command.Run()
}
