//go:build !linux

package main

import (
	"context"
	"errors"
)

func newApplianceProvider(context.Context, string) (networkProvider, error) {
	return nil, errors.New("the appliance profile requires Linux and an explicitly installed privileged appliance helper")
}
