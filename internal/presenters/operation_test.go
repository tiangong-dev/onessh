package presenters

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRenderBatchResultsPreservesOutput(t *testing.T) {
	t.Parallel()

	var out, errOut bytes.Buffer
	anyFailed := RenderBatchResults(&out, &errOut, []string{"ok", "skip", "output-fail", "plain-fail"}, []BatchResult{
		{Alias: "ok"},
		{Alias: "skip", Skip: true, Err: errors.New("missing identity")},
		{
			Alias:  "output-fail",
			Stdout: []byte("remote stdout\n"),
			Stderr: []byte("remote stderr\n"),
			Err:    errors.New("ssh failed"),
		},
		{Alias: "plain-fail", Err: errors.New("timeout")},
	})
	if !anyFailed {
		t.Fatalf("RenderBatchResults returned anyFailed=false")
	}

	wantOut := strings.Join([]string{
		"ok                    OK",
		"=== output-fail ===",
		"remote stdout",
		"plain-fail            FAIL",
		"",
	}, "\n")
	if got := out.String(); got != wantOut {
		t.Fatalf("stdout:\nwant:\n%s\ngot:\n%s", wantOut, got)
	}

	wantErr := strings.Join([]string{
		"SKIP skip: missing identity",
		"remote stderr",
		"FAIL output-fail: ssh failed",
		"",
	}, "\n")
	if got := errOut.String(); got != wantErr {
		t.Fatalf("stderr:\nwant:\n%s\ngot:\n%s", wantErr, got)
	}
}

func TestRenderDryRunHostsPreservesOutput(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := RenderDryRunHosts(&out, []DryRunHost{
		{Alias: "prod", Host: "prod.example.com", User: "alice", Port: 22},
		{Alias: "skip", Host: "skip.example.com", SkipError: "missing identity"},
	})
	if err != nil {
		t.Fatalf("RenderDryRunHosts: %v", err)
	}

	want := strings.Join([]string{
		"Matched 2 host(s):",
		"  prod                 alice@prod.example.com:22",
		"  skip                 skip.example.com (SKIP: missing identity)",
		"",
	}, "\n")
	if got := out.String(); got != want {
		t.Fatalf("dry-run output:\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestRenderDryRunCommandAndUploadPreserveOutput(t *testing.T) {
	t.Parallel()

	t.Run("command", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		if err := RenderDryRunCommand(&out, []string{"uptime", "-p"}); err != nil {
			t.Fatalf("RenderDryRunCommand: %v", err)
		}
		if got, want := out.String(), "Command: uptime -p\n"; got != want {
			t.Fatalf("command output = %q, want %q", got, want)
		}
	})

	t.Run("upload", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		if err := RenderDryRunUpload(&out, []string{"app.conf", "deploy.sh"}, "/tmp/app"); err != nil {
			t.Fatalf("RenderDryRunUpload: %v", err)
		}
		if got, want := out.String(), "Upload: app.conf, deploy.sh -> :/tmp/app\n"; got != want {
			t.Fatalf("upload output = %q, want %q", got, want)
		}
	})
}

func TestRenderAuditLogPreservesOutput(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		if err := RenderAuditLog(&out, nil, "table"); err != nil {
			t.Fatalf("RenderAuditLog empty: %v", err)
		}
		if got, want := out.String(), "No audit log entries.\n"; got != want {
			t.Fatalf("empty output = %q, want %q", got, want)
		}
	})

	t.Run("table", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		err := RenderAuditLog(&out, []AuditLogEvent{
			{
				Time:   "2026-03-06T12:34:56Z",
				Action: "connect",
				Alias:  "prod",
				Host:   "prod.example.com",
				User:   "alice",
				Result: "ok",
			},
			{
				Time:   "2026-03-06T12:35:56Z",
				Action: "exec",
				Result: "fail",
				Error:  "timeout",
			},
		}, "table")
		if err != nil {
			t.Fatalf("RenderAuditLog table: %v", err)
		}

		want := strings.Join([]string{
			"TIME                  ACTION   ALIAS  HOST              USER   RESULT  ERROR",
			"2026-03-06T12:34:56Z  connect  prod   prod.example.com  alice  ok      -",
			"2026-03-06T12:35:56Z  exec     -      -                 -      fail    timeout",
			"",
		}, "\n")
		if got := out.String(); got != want {
			t.Fatalf("table output:\nwant:\n%s\ngot:\n%s", want, got)
		}
	})

	t.Run("json", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		err := RenderAuditLog(&out, []AuditLogEvent{
			{
				Time:   "2026-03-06T12:34:56Z",
				Action: "connect",
				Alias:  "prod",
				Host:   "prod.example.com",
				User:   "alice",
				Result: "ok",
			},
		}, "json")
		if err != nil {
			t.Fatalf("RenderAuditLog json: %v", err)
		}

		want := strings.Join([]string{
			"[",
			"  {",
			"    \"time\": \"2026-03-06T12:34:56Z\",",
			"    \"action\": \"connect\",",
			"    \"alias\": \"prod\",",
			"    \"host\": \"prod.example.com\",",
			"    \"user\": \"alice\",",
			"    \"result\": \"ok\"",
			"  }",
			"]",
			"",
		}, "\n")
		if got := out.String(); got != want {
			t.Fatalf("json output:\nwant:\n%s\ngot:\n%s", want, got)
		}
	})
}
