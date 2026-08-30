//go:build windows

package supervisor

import "os/exec"

func configureProcess(command *exec.Cmd) {}

// terminateProcess kills only the direct runner process in Increment 1.
// A future Windows delivery must assign runners to a Job Object so child
// processes receive the same ownership and termination guarantee.
func terminateProcess(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	return command.Process.Kill()
}
