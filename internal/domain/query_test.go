package domain

import (
	"reflect"
	"testing"
)

func TestMatchHostFilter(t *testing.T) {
	t.Parallel()

	host := HostQuery{
		Address:     "db01.example.com",
		Description: "primary database",
	}

	cases := map[string]bool{
		"prod-*":   true,
		"db*.com":  true,
		"*database": true,
		"web-*":    false,
	}

	for pattern, want := range cases {
		if got := MatchHostFilter("prod-db", host, pattern); got != want {
			t.Fatalf("MatchHostFilter(%q) = %v, want %v", pattern, got, want)
		}
	}
}

func TestCollectFilteredHosts(t *testing.T) {
	t.Parallel()

	hosts := map[string]HostQuery{
		"prod-db": {
			Address:     "db01.example.com",
			Description: "primary database",
			Tags:        []string{"prod", "db"},
		},
		"prod-web": {
			Address: "web01.example.com",
			Tags:    []string{"prod", "web"},
		},
		"dev-db": {
			Address: "dev-db.example.com",
			Tags:    []string{"dev", "db"},
		},
	}

	got := CollectFilteredHosts(hosts, "prod", "*db*")
	want := []string{"prod-db"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("CollectFilteredHosts mismatch: want=%v got=%v", want, got)
	}

	got = CollectFilteredHosts(hosts, "db", "")
	want = []string{"dev-db", "prod-db"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("CollectFilteredHosts tag mismatch: want=%v got=%v", want, got)
	}
}
