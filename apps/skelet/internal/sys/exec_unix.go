//go:build unix

// Package sys holds the process-level calls the commands share
package sys

import "syscall"

// Exec replaces this process, exactly like bash's exec: the tty, signals and exit code all belong to the new program
func Exec(path string, argv, env []string) error {
	return syscall.Exec(path, argv, env)
}
