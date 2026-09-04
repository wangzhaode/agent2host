package daemon

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/wangzhaode/agent2host/internal/app"
	"github.com/wangzhaode/agent2host/internal/model"
)

func TestDaemonRoundTrip(t *testing.T) {
	root := t.TempDir()
	service, err := app.New(filepath.Join(root, "config.json"), filepath.Join(root, "ssh"))
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(root, "daemon.sock")
	server := NewServer(service, socket)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()

	client := &Client{Socket: socket}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := client.Call(MethodPing, struct{}{}, nil); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("daemon did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}

	device := model.Host{Name: "test", Target: "test-host", Root: "/tmp/work"}
	if err := client.Call(MethodHostAdd, device, nil); err != nil {
		t.Fatal(err)
	}
	var devices []model.Host
	if err := client.Call(MethodHostList, struct{}{}, &devices); err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 || devices[1] != device {
		t.Fatalf("devices = %#v", devices)
	}

	var result model.ExecResult
	if err := client.Call(MethodExec, model.ExecRequest{Host: model.LocalHost, Command: "printf daemon-ok"}, &result); err != nil {
		t.Fatal(err)
	}
	if result.Output != "daemon-ok" || result.ExitCode != 0 {
		t.Fatalf("exec result = %#v", result)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop")
	}
}
