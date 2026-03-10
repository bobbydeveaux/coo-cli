package runtime

import (
	"fmt"
	"testing"
)

func TestIsRemoteExitError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"exit code message", fmt.Errorf("command terminated with exit code 1"), true},
		{"exit code 0", fmt.Errorf("command terminated with exit code 0"), true},
		{"transport error", fmt.Errorf("connection refused"), false},
		{"unrelated", fmt.Errorf("some other error"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isRemoteExitError(tc.err)
			if got != tc.want {
				t.Errorf("isRemoteExitError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestK8sRuntime_TypeReturnsK8s ensures K8sRuntime.Type() == RuntimeK8s.
func TestK8sRuntime_TypeReturnsK8s(t *testing.T) {
	r := &K8sRuntime{}
	if r.Type() != RuntimeK8s {
		t.Errorf("Type() = %q, want %q", r.Type(), RuntimeK8s)
	}
}
