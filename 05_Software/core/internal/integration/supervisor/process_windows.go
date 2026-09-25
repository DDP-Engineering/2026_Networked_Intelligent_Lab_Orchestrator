//go:build windows

package supervisor

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureProcess(command *exec.Cmd) {}

// terminateProcess kills only the direct runner process in Increment 1.
// A future Windows delivery must assign runners to a Job Object so child
// processes receive the same ownership and termination guarantee.
func terminateProcess(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	err := command.Process.Kill()
	if err == nil || errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.EINVAL) {
		return nil
	}
	return err
}
