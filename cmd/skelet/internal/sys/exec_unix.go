//go:build unix

// Package sys holds the process-level calls the commands share
package sys

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// Exec replaces this process, exactly like bash's exec: the tty, signals and exit code all belong to the new program
func Exec(path string, argv, env []string) error {
	return syscall.Exec(path, argv, env)
}

// ReplaceIn is Replace from dir, for a command that has to run where its project is - pnpm in an app's directory
func ReplaceIn(dir, path string, argv, env []string) int {
	if err := os.Chdir(dir); err != nil {
		fmt.Fprintf(os.Stderr, "skelet: %v\n", err)
		return 1
	}
	return Replace(path, argv, env)
}

// Replace is Exec for a command's last step. It only returns when the exec failed, with the code a shell would give
func Replace(path string, argv, env []string) int {
	err := Exec(path, argv, env)
	fmt.Fprintf(os.Stderr, "skelet: %s: %v\n", path, err)
	if errors.Is(err, fs.ErrNotExist) {
		return 127
	}
	return 126
}
