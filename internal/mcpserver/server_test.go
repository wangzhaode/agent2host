package mcpserver

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wangzhaode/agent2host/internal/app"
	"github.com/wangzhaode/agent2host/internal/daemon"
)

func TestMCPListsAndCallsTools(t *testing.T) {
	root := t.TempDir()
	service, err := app.New(filepath.Join(root, "config.json"), filepath.Join(root, "ssh"))
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(root, "daemon.sock")
	daemonServer := daemon.NewServer(service, socket)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go daemonServer.Serve(ctx)
	waitForDaemon(t, socket)

	bridge := New(&daemon.Client{Socket: socket})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverDone := make(chan error, 1)
	go func() { serverDone <- bridge.server.Run(ctx, serverTransport) }()
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "agent2host-test", Version: "1"}, nil)
	session, err := mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 22 {
		t.Fatalf("tool count = %d, want 22", len(tools.Tools))
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "host_list", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("host_list returned error: %#v", result.Content)
	}
}

func TestMCPStdioLauncher(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	service, err := app.New(configPath, filepath.Join(root, "ssh"))
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(root, "daemon.sock")
	daemonServer := daemon.NewServer(service, socket)
	daemonContext, stopDaemon := context.WithCancel(context.Background())
	defer stopDaemon()
	go daemonServer.Serve(daemonContext)
	waitForDaemon(t, socket)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.Command(filepath.Join(repoRoot, "scripts", "launch_agent2host_mcp"))
	command.Dir = t.TempDir()
	command.Env = append(os.Environ(), "AGENT2HOST_SOCKET="+socket, "AGENT2HOST_CONFIG="+configPath)
	client := mcp.NewClient(&mcp.Implementation{Name: "agent2host-stdio-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 22 {
		t.Fatalf("tool count = %d, want 22", len(tools.Tools))
	}
}

func waitForDaemon(t *testing.T, socket string) {
	t.Helper()
	client := &daemon.Client{Socket: socket}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := client.Call(daemon.MethodPing, struct{}{}, nil); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("daemon did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
