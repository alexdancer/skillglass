//go:build !darwin

package setup

import "errors"

func installSetup(home, destination string) error {
	return errors.New("companion setup writer is validated on macOS only")
}
