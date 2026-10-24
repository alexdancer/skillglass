//go:build !darwin

package setup

import "errors"

import "github.com/alexdancer/skillglass/internal/companion"

func installSetup(home, destination string, bundle companion.Bundle) error {
	_ = bundle
	return errors.New("companion setup writer is validated on macOS only")
}
