package sshx

import (
	"slices"
	"strings"
	"testing"

	"github.com/wangzhaode/agent2host/internal/model"
)

func TestShellQuote(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{"", "''"},
		{"hello world", "'hello world'"},
		{"it's here", "'it'\\''s here'"},
	} {
		if got := ShellQuote(test.input); got != test.want {
			t.Errorf("ShellQuote(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestRemotePayload(t *testing.T) {
	host := model.Host{Name: "dev", Target: "dev-host", Root: "/srv/my app", Shell: "/bin/bash"}
	got := RemotePayload(host, "printf '%s' ok", "")
	want := "cd -- '/srv/my app' && exec '/bin/bash' -lc 'printf '\\''%s'\\'' ok'"
	if got != want {
		t.Fatalf("RemotePayload() = %q, want %q", got, want)
	}
}

func TestInteractiveSSHOptionsPrecedeTarget(t *testing.T) {
	transport, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	host := model.Host{Name: "dev", Target: "dev-host"}
	args := transport.InteractiveCommand(host, "").Args
	target := slices.Index(args, host.Target)
	tty := slices.Index(args, "-tt")
	if tty < 0 || target < 0 || tty > target {
		t.Fatalf("unexpected ssh args: %s", strings.Join(args, " "))
	}
}
