//go:build !linux && !darwin && !windows

package operatorcontrols

import "os"

func tryImportLock(_ *os.File) (bool, error) { return false, ErrAdmissionUnavailable }
