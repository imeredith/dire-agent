//go:build !darwin && !linux

package lifecycle

import (
	"context"
	"errors"
	"os/exec"
)

type fileLock struct{}

func acquireFileLock(context.Context, string) (*fileLock, error) {
	return nil, errors.New("daemon lifecycle is supported only on macOS and Linux")
}

func (lock *fileLock) close() error { return nil }

func configureDetached(*exec.Cmd) error {
	return errors.New("daemon lifecycle is supported only on macOS and Linux")
}

func osProcessAlive(int) (bool, error) {
	return false, errors.New("daemon lifecycle is supported only on macOS and Linux")
}
