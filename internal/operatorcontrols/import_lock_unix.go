//go:build linux || darwin

package operatorcontrols

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func tryImportLock(file *os.File) (bool, error) {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}
