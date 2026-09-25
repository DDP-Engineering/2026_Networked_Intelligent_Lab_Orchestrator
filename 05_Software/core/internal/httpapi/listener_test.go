package httpapi

import "testing"

func TestValidateSimulationBindRequiresLiteralLoopbackHost(t *testing.T) {
	t.Parallel()

	for _, address := range []string{"127.0.0.1:8080", "127.42.0.9:0", "[::1]:8080"} {
		address := address
		t.Run("accept_"+address, func(t *testing.T) {
			t.Parallel()
			if err := ValidateSimulationBind(address); err != nil {
				t.Fatalf("ValidateSimulationBind(%q): %v", address, err)
			}
		})
	}

	for _, address := range []string{
		":8080",
		"0.0.0.0:8080",
		"[::]:8080",
		"192.168.1.20:8080",
		"localhost:8080",
		"127.0.0.1",
		"not-an-address",
	} {
		address := address
		t.Run("reject_"+address, func(t *testing.T) {
			t.Parallel()
			if err := ValidateSimulationBind(address); err == nil {
				t.Fatalf("ValidateSimulationBind(%q) succeeded", address)
			}
		})
	}
}
