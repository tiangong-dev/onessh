package domain

import (
	"reflect"
	"testing"
)

func TestValidateEnvKey(t *testing.T) {
	t.Parallel()

	valid := []string{"FOO", "_FOO", "FOO_1", "Path"}
	for _, key := range valid {
		if err := ValidateEnvKey(key); err != nil {
			t.Fatalf("ValidateEnvKey(%q): %v", key, err)
		}
	}

	invalid := []string{"", "1FOO", "FOO-BAR", "FOO.BAR"}
	for _, key := range invalid {
		if err := ValidateEnvKey(key); err == nil {
			t.Fatalf("ValidateEnvKey(%q) expected error", key)
		}
	}
}

func TestParseEnvAssignments(t *testing.T) {
	t.Parallel()

	got, err := ParseEnvAssignments([]string{"FOO=bar", "EMPTY=", "PATH=/usr/bin"})
	if err != nil {
		t.Fatalf("ParseEnvAssignments: %v", err)
	}

	want := map[string]string{
		"FOO":   "bar",
		"EMPTY": "",
		"PATH":  "/usr/bin",
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected env map: want=%v got=%v", want, got)
	}
}

func TestParseEnvAssignmentsInvalid(t *testing.T) {
	t.Parallel()

	cases := [][]string{
		{""},
		{"1FOO=bar"},
		{"FOO"},
	}
	for _, values := range cases {
		if _, err := ParseEnvAssignments(values); err == nil {
			t.Fatalf("ParseEnvAssignments(%v) expected error", values)
		}
	}
}

func TestParseEnvKeys(t *testing.T) {
	t.Parallel()

	got, err := ParseEnvKeys([]string{" FOO ", "BAR_1"})
	if err != nil {
		t.Fatalf("ParseEnvKeys: %v", err)
	}
	want := []string{"FOO", "BAR_1"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected keys: want=%v got=%v", want, got)
	}
}

func TestParseEnvKeysInvalid(t *testing.T) {
	t.Parallel()

	cases := [][]string{
		{""},
		{"1FOO"},
		{"FOO-BAR"},
	}
	for _, values := range cases {
		if _, err := ParseEnvKeys(values); err == nil {
			t.Fatalf("ParseEnvKeys(%v) expected error", values)
		}
	}
}
