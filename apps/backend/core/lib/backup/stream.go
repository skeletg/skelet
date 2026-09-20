package backup

import (
	"fmt"
	"io"
	"os/exec"
)

// The ordering is load-bearing - when the upload fails before draining the pipe the child is still blocked writing into it, so it must be killed and the pipe drained BEFORE cmd.Wait, or Wait blocks forever on a child that can never exit; stream_test.go pins this
func streamCommand(cmd *exec.Cmd, upload func(io.Reader) error) (uploadErr, runErr error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		stdout.Close() // Wait never runs, so the pipe won't be closed for us
		return nil, fmt.Errorf("start %s: %w", cmd.Path, err)
	}

	uploadErr = upload(stdout)
	if uploadErr != nil {
		_ = cmd.Process.Kill()
		_, _ = io.Copy(io.Discard, stdout)
	}
	if err := cmd.Wait(); err != nil {
		runErr = fmt.Errorf("%s: %w", cmd.Path, err)
	}
	return uploadErr, runErr
}
