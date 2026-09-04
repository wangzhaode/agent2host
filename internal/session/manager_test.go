package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wangzhaode/agent2host/internal/model"
	"github.com/wangzhaode/agent2host/internal/sshx"
)

func TestPersistentLocalSession(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	transport, err := sshx.New(filepath.Join(root, "controls"))
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(transport)
	host := model.Host{Name: model.LocalHost, Root: root, Shell: "/bin/sh"}
	info, err := manager.Open(host, model.OpenSessionRequest{Host: model.LocalHost})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close(info.ID)

	result := execForTest(t, manager, info.ID, "printf 'hello-from-pty'")
	if result.Output != "hello-from-pty" || result.ExitCode != 0 {
		t.Fatalf("first exec = %#v", result)
	}
	execForTest(t, manager, info.ID, "cd subdir")
	result = execForTest(t, manager, info.ID, "pwd")
	if !strings.HasSuffix(result.Output, "/subdir") {
		t.Fatalf("persistent pwd = %q", result.Output)
	}
	execForTest(t, manager, info.ID, "export AGENT2HOST_TEST=kept")
	result = execForTest(t, manager, info.ID, "printf '%s' \"$AGENT2HOST_TEST\"")
	if result.Output != "kept" {
		t.Fatalf("persistent environment = %q", result.Output)
	}
}

func TestSessionExecReturnsExitStatus(t *testing.T) {
	transport, err := sshx.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(transport)
	info, err := manager.Open(model.Host{Name: model.LocalHost, Shell: "/bin/sh"}, model.OpenSessionRequest{Host: model.LocalHost})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close(info.ID)
	result := execForTest(t, manager, info.ID, "printf failure; false")
	if result.Output != "failure" || result.ExitCode == 0 {
		t.Fatalf("exec = %#v", result)
	}
}

func execForTest(t *testing.T, manager *Manager, id, command string) model.SessionExecResult {
	t.Helper()
	result, err := manager.Exec(id, command, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if result.TimedOut {
		t.Fatalf("command timed out: %s; output %q", command, result.Output)
	}
	return result
}
