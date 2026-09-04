package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wangzhaode/agent2host/internal/buildinfo"
	"github.com/wangzhaode/agent2host/internal/daemon"
	"github.com/wangzhaode/agent2host/internal/model"
)

type Server struct {
	server *mcp.Server
}

type emptyInput struct{}

type hostListOutput struct {
	Hosts []model.Host `json:"hosts"`
}

type androidListOutput struct {
	Targets []model.AndroidTarget `json:"targets"`
}

type okOutput struct {
	OK bool `json:"ok"`
}

type sessionListOutput struct {
	Sessions []model.SessionInfo `json:"sessions"`
}

type sessionWriteOutput struct {
	Bytes int `json:"bytes"`
}

func New(client *daemon.Client) *Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "agent2host", Version: buildinfo.Version}, nil)
	registerHostTools(server, client)
	registerShellTools(server, client)
	registerFileTools(server, client)
	registerAndroidTools(server, client)
	return &Server{server: server}
}

func (s *Server) Run(ctx context.Context) error {
	return s.server.Run(ctx, &mcp.StdioTransport{})
}

func registerHostTools(server *mcp.Server, client *daemon.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "host_list", Description: "List the built-in local host and configured SSH hosts. Use this before choosing where a command or Android operation runs.",
		Annotations: readOnly("List hosts"),
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, hostListOutput, error) {
		var hosts []model.Host
		err := client.Call(daemon.MethodHostList, emptyInput{}, &hosts)
		return nil, hostListOutput{Hosts: hosts}, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "host_add", Description: "Add or update a host using an OpenSSH alias or user@host target. The remote host only needs SSH and a POSIX shell.",
		Annotations: writeHint("Add host", false),
	}, func(_ context.Context, _ *mcp.CallToolRequest, input model.Host) (*mcp.CallToolResult, okOutput, error) {
		err := client.Call(daemon.MethodHostAdd, input, nil)
		return nil, okOutput{OK: err == nil}, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "host_remove", Description: "Remove a saved SSH host definition. Existing sessions remain alive until closed.",
		Annotations: writeHint("Remove host", true),
	}, func(_ context.Context, _ *mcp.CallToolRequest, input daemon.NameParam) (*mcp.CallToolResult, okOutput, error) {
		err := client.Call(daemon.MethodHostRemove, input, nil)
		return nil, okOutput{OK: err == nil}, err
	})
}

func registerShellTools(server *mcp.Server, client *daemon.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "shell_exec", Description: "Run one non-interactive command on a host over a multiplexed SSH connection. Prefer session_open plus session_exec when several commands share shell state.",
		Annotations: openWorld("Run command", true),
	}, func(_ context.Context, _ *mcp.CallToolRequest, input model.ExecRequest) (*mcp.CallToolResult, model.ExecResult, error) {
		var output model.ExecResult
		err := client.Call(daemon.MethodExec, input, &output)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "session_open", Description: "Open a persistent native PTY shell on a host. Reuse its session_id for related work so cwd, environment, jobs, and interactive programs persist.",
		Annotations: openWorld("Open shell session", true),
	}, func(_ context.Context, _ *mcp.CallToolRequest, input model.OpenSessionRequest) (*mcp.CallToolResult, model.SessionInfo, error) {
		var output model.SessionInfo
		err := client.Call(daemon.MethodSessionOpen, input, &output)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "session_list", Description: "List persistent shell sessions currently held by the local Agent2Host daemon.",
		Annotations: readOnly("List shell sessions"),
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, sessionListOutput, error) {
		var sessions []model.SessionInfo
		err := client.Call(daemon.MethodSessionList, emptyInput{}, &sessions)
		return nil, sessionListOutput{Sessions: sessions}, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "session_exec", Description: "Execute a command inside an existing persistent shell and wait for its exit status. Shell state changes persist. For prompts or full-screen programs, use session_write and session_read instead.",
		Annotations: openWorld("Run in shell session", true),
	}, func(_ context.Context, _ *mcp.CallToolRequest, input daemon.SessionExecParam) (*mcp.CallToolResult, model.SessionExecResult, error) {
		var output model.SessionExecResult
		err := client.Call(daemon.MethodSessionExec, input, &output)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "session_read", Description: "Read PTY output from a cursor. Pass the returned cursor on the next call; wait_ms can long-poll for new output.",
		Annotations: readOnly("Read shell output"),
	}, func(_ context.Context, _ *mcp.CallToolRequest, input daemon.SessionReadParam) (*mcp.CallToolResult, model.SessionReadResult, error) {
		var output model.SessionReadResult
		err := client.Call(daemon.MethodSessionRead, input, &output)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "session_write", Description: "Write raw text or control bytes to a persistent PTY. Include a newline when submitting terminal input.",
		Annotations: openWorld("Write shell input", true),
	}, func(_ context.Context, _ *mcp.CallToolRequest, input daemon.SessionWriteParam) (*mcp.CallToolResult, sessionWriteOutput, error) {
		var output sessionWriteOutput
		err := client.Call(daemon.MethodSessionWrite, input, &output)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "session_resize", Description: "Resize a persistent PTY before using terminal applications whose layout depends on rows and columns.",
		Annotations: writeHint("Resize shell session", false),
	}, func(_ context.Context, _ *mcp.CallToolRequest, input daemon.SessionResizeParam) (*mcp.CallToolResult, okOutput, error) {
		err := client.Call(daemon.MethodSessionResize, input, nil)
		return nil, okOutput{OK: err == nil}, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "session_close", Description: "Close and forget a persistent shell session.",
		Annotations: writeHint("Close shell session", true),
	}, func(_ context.Context, _ *mcp.CallToolRequest, input daemon.SessionIDParam) (*mcp.CallToolResult, okOutput, error) {
		err := client.Call(daemon.MethodSessionClose, input, nil)
		return nil, okOutput{OK: err == nil}, err
	})
}

func registerFileTools(server *mcp.Server, client *daemon.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "file_read", Description: "Read a UTF-8 text file from a host. Relative paths resolve under the host root.",
		Annotations: readOnly("Read host file"),
	}, func(_ context.Context, _ *mcp.CallToolRequest, input model.FileReadRequest) (*mcp.CallToolResult, model.FileReadResult, error) {
		var output model.FileReadResult
		err := client.Call(daemon.MethodFileRead, input, &output)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "file_write", Description: "Atomically write a UTF-8 text file on a host. Existing permissions are preserved when possible.",
		Annotations: openWorld("Write host file", true),
	}, func(_ context.Context, _ *mcp.CallToolRequest, input model.FileWriteRequest) (*mcp.CallToolResult, model.FileWriteResult, error) {
		var output model.FileWriteResult
		err := client.Call(daemon.MethodFileWrite, input, &output)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "file_replace", Description: "Replace an exact text span in a host file and fail on ambiguous matches unless replace_all is true.",
		Annotations: openWorld("Edit host file", true),
	}, func(_ context.Context, _ *mcp.CallToolRequest, input model.FileReplaceRequest) (*mcp.CallToolResult, model.FileReplaceResult, error) {
		var output model.FileReplaceResult
		err := client.Call(daemon.MethodFileReplace, input, &output)
		return nil, output, err
	})
}

func registerAndroidTools(server *mcp.Server, client *daemon.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "android_list", Description: "List Android devices and emulators visible to the adb server on a host, including offline and unauthorized targets. Never guess a serial when multiple targets exist.",
		Annotations: readOpenWorld("List Android targets"),
	}, func(_ context.Context, _ *mcp.CallToolRequest, input model.AndroidListRequest) (*mcp.CallToolResult, androidListOutput, error) {
		var targets []model.AndroidTarget
		err := client.Call(daemon.MethodAndroidList, input, &targets)
		return nil, androidListOutput{Targets: targets}, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "android_exec", Description: "Run a non-interactive shell command on one Android target. adb runs on the selected host and the command is supplied over stdin to avoid nested shell quoting.",
		Annotations: openWorld("Run Android command", true),
	}, func(_ context.Context, _ *mcp.CallToolRequest, input model.AndroidExecRequest) (*mcp.CallToolResult, model.ExecResult, error) {
		var output model.ExecResult
		err := client.Call(daemon.MethodAndroidExec, input, &output)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "android_session_open", Description: "Open a persistent interactive adb shell on an Android target. Continue with session_read, session_write, session_resize, and session_close.",
		Annotations: openWorld("Open Android shell", true),
	}, func(_ context.Context, _ *mcp.CallToolRequest, input model.AndroidOpenSessionRequest) (*mcp.CallToolResult, model.SessionInfo, error) {
		var output model.SessionInfo
		err := client.Call(daemon.MethodAndroidSessionOpen, input, &output)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "android_push", Description: "Push one file from the Agent2Host machine to an Android target. For an SSH host, the file is streamed through a protected temporary host file and removed afterward.",
		Annotations: openWorld("Push Android file", true),
	}, func(_ context.Context, _ *mcp.CallToolRequest, input model.AndroidPushRequest) (*mcp.CallToolResult, model.AndroidTransferResult, error) {
		var output model.AndroidTransferResult
		err := client.Call(daemon.MethodAndroidPush, input, &output)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "android_pull", Description: "Pull one Android file to the Agent2Host machine using an atomic local replacement. local_path must be absolute.",
		Annotations: openWorld("Pull Android file", true),
	}, func(_ context.Context, _ *mcp.CallToolRequest, input model.AndroidPullRequest) (*mcp.CallToolResult, model.AndroidTransferResult, error) {
		var output model.AndroidTransferResult
		err := client.Call(daemon.MethodAndroidPull, input, &output)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "android_install", Description: "Stream a local APK through the selected host and install it on one Android target. The temporary host copy is protected and always removed.",
		Annotations: openWorld("Install Android APK", true),
	}, func(_ context.Context, _ *mcp.CallToolRequest, input model.AndroidInstallRequest) (*mcp.CallToolResult, model.AndroidTransferResult, error) {
		var output model.AndroidTransferResult
		err := client.Call(daemon.MethodAndroidInstall, input, &output)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "android_logcat", Description: "Return a bounded snapshot of recent logcat output from one Android target. Use filters for focused diagnostics.",
		Annotations: readOpenWorld("Read Android logs"),
	}, func(_ context.Context, _ *mcp.CallToolRequest, input model.AndroidLogcatRequest) (*mcp.CallToolResult, model.ExecResult, error) {
		var output model.ExecResult
		err := client.Call(daemon.MethodAndroidLogcat, input, &output)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "android_screenshot", Description: "Capture the current Android display as a PNG on the Agent2Host machine. local_path must be absolute.",
		Annotations: openWorld("Capture Android screenshot", true),
	}, func(_ context.Context, _ *mcp.CallToolRequest, input model.AndroidScreenshotRequest) (*mcp.CallToolResult, model.AndroidTransferResult, error) {
		var output model.AndroidTransferResult
		err := client.Call(daemon.MethodAndroidScreenshot, input, &output)
		return nil, output, err
	})
}

func readOnly(title string) *mcp.ToolAnnotations {
	closed := false
	return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, OpenWorldHint: &closed}
}

func readOpenWorld(title string) *mcp.ToolAnnotations {
	open := true
	return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, OpenWorldHint: &open}
}

func writeHint(title string, destructive bool) *mcp.ToolAnnotations {
	closed := false
	return &mcp.ToolAnnotations{Title: title, DestructiveHint: &destructive, OpenWorldHint: &closed}
}

func openWorld(title string, destructive bool) *mcp.ToolAnnotations {
	open := true
	return &mcp.ToolAnnotations{Title: title, DestructiveHint: &destructive, OpenWorldHint: &open}
}
