package cli

import (
	"reflect"
	"testing"

	"onessh/internal/store"
)

func TestNormalizeUserAlias(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		" Ops_User ": "ops_user",
		"Web-01":     "web-01",
		"__bad__":    "bad",
		"??":         "",
	}

	for input, want := range cases {
		if got := normalizeUserAlias(input); got != want {
			t.Fatalf("normalizeUserAlias(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSortedUserAliases(t *testing.T) {
	t.Parallel()

	users := map[string]store.UserConfig{
		"ops":  {Name: "ubuntu"},
		"dev":  {Name: "debian"},
		"root": {Name: "root"},
	}

	got := sortedUserAliases(users)
	want := []string{"dev", "ops", "root"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sortedUserAliases() = %v, want %v", got, want)
	}
}

func TestFindUserAliasByName(t *testing.T) {
	t.Parallel()

	users := map[string]store.UserConfig{
		"ops": {Name: "Ubuntu"},
		"dev": {Name: "debian"},
	}

	if got := findUserAliasByName(users, " ubuntu "); got != "ops" {
		t.Fatalf("findUserAliasByName() = %q, want ops", got)
	}
	if got := findUserAliasByName(users, "missing"); got != "" {
		t.Fatalf("findUserAliasByName() = %q, want empty", got)
	}
}
