package domain

import (
	"reflect"
	"testing"
)

func TestNormalizeTags(t *testing.T) {
	t.Parallel()

	got := NormalizeTags([]string{" Prod ", "db", "prod", "", "API"})
	want := []string{"api", "db", "prod"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("NormalizeTags mismatch: want=%v got=%v", want, got)
	}
}

func TestHostHasTagKeepsStoredTagCaseBehavior(t *testing.T) {
	t.Parallel()

	tags := []string{"Prod", "db"}
	if HostHasTag(tags, "prod") {
		t.Fatalf("HostHasTag should not normalize stored tag case")
	}
	if !HostHasTag(tags, " db ") {
		t.Fatalf("HostHasTag should trim and lowercase query tag")
	}
}
