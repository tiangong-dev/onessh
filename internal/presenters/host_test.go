package presenters

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderHostListJSON(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := RenderHostListJSON(&out, []HostListRow{
		{
			Alias:     "prod",
			Desc:      "production",
			Host:      "prod.example.com",
			User:      "alice",
			UserRef:   "alice",
			Auth:      "key",
			Port:      22,
			ProxyJump: "-",
			Tags:      "prod,web",
			Status:    "ok",
		},
	})
	if err != nil {
		t.Fatalf("RenderHostListJSON returned error: %v", err)
	}

	want := strings.Join([]string{
		"[",
		"  {",
		"    \"alias\": \"prod\",",
		"    \"desc\": \"production\",",
		"    \"host\": \"prod.example.com\",",
		"    \"user\": \"alice\",",
		"    \"user_ref\": \"alice\",",
		"    \"auth\": \"key\",",
		"    \"port\": 22,",
		"    \"proxy_jump\": \"-\",",
		"    \"tags\": \"prod,web\",",
		"    \"status\": \"ok\"",
		"  }",
		"]",
		"",
	}, "\n")
	if got := out.String(); got != want {
		t.Fatalf("unexpected json output:\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestRenderHostListTable(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := RenderHostListTable(&out, []HostListRow{
		{
			Alias:     "prod",
			Desc:      "production",
			Host:      "prod.example.com",
			User:      "alice",
			UserRef:   "alice",
			Auth:      "key",
			Port:      22,
			ProxyJump: "-",
			Tags:      "prod,web",
			Status:    "ok",
		},
	})
	if err != nil {
		t.Fatalf("RenderHostListTable returned error: %v", err)
	}

	want := strings.Join([]string{
		"ALIAS  DESC        HOST              USER   USER_REF  AUTH  PORT  PROXY_JUMP  TAGS      STATUS",
		"prod   production  prod.example.com  alice  alice     key   22    -           prod,web  ok",
		"",
	}, "\n")
	if got := out.String(); got != want {
		t.Fatalf("unexpected table output:\nwant:\n%s\ngot:\n%s", want, got)
	}
}
