//go:build !linux

package main

import (
	"context"
	"errors"
)

func serve(context.Context, options) error {
	return errors.New("network helper is supported only on Linux")
}

func recoverOnce(context.Context, string) error {
	return errors.New("network recovery is supported only on Linux")
}
