//go:build linux

package nethelper

import (
	"testing"
	"unsafe"
)

func TestGPIOV2RequestMatchesLinuxABI(t *testing.T) {
	var request gpioV2LineRequest
	if got := unsafe.Sizeof(request); got != 592 {
		t.Fatalf("request size = %d, want 592", got)
	}
	if got := unsafe.Offsetof(request.Padding); got != 568 {
		t.Fatalf("padding offset = %d, want 568", got)
	}
	if got := unsafe.Offsetof(request.FD); got != 588 {
		t.Fatalf("fd offset = %d, want 588", got)
	}
}
