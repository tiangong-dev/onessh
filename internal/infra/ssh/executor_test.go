package ssh

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestRunExternalCommandRejectsUnsupportedCommand(t *testing.T) {
	t.Parallel()

	err := RunExternalCommand(context.Background(), "sh", []string{"-c", "true"}, nil, nil, nil, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "unsupported external command") {
		t.Fatalf("expected unsupported command error, got %v", err)
	}
}
