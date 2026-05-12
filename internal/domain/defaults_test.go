package domain

import "testing"

func TestEffectivePort(t *testing.T) {
	t.Parallel()

	if DefaultSSHPort != 22 {
		t.Fatalf("DefaultSSHPort = %d, want 22", DefaultSSHPort)
	}

	cases := map[int]int{
		0:    22,
		-1:   22,
		1:    1,
		2222: 2222,
	}

	for input, want := range cases {
		if got := EffectivePort(input); got != want {
			t.Fatalf("EffectivePort(%d) = %d, want %d", input, got, want)
		}
	}
}
