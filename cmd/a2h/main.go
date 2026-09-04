package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"golang.org/x/term"

	agent2host "github.com/wangzhaode/agent2host"
	"github.com/wangzhaode/agent2host/internal/app"
	"github.com/wangzhaode/agent2host/internal/buildinfo"
	"github.com/wangzhaode/agent2host/internal/config"
	"github.com/wangzhaode/agent2host/internal/daemon"
	"github.com/wangzhaode/agent2host/internal/integration"
	"github.com/wangzhaode/agent2host/internal/mcpserver"
	"github.com/wangzhaode/agent2host/internal/model"
	"github.com/wangzhaode/agent2host/internal/sshx"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "a2h:", err)
		var status exitStatusError
		if errors.As(err, &status) && status.code > 0 && status.code < 126 {
			os.Exit(status.code)
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	switch args[0] {
	case "help", "-h", "--help":
		usage()
		return nil
	case "version", "--version":
		fmt.Println(buildinfo.Version)
		return nil
	case "daemon":
		return runDaemon(args[1:])
	case "mcp":
		client, err := newClient(true)
		if err != nil {
			return err
		}
		return mcpserver.New(client).Run(context.Background())
	case "host", "device":
		return runHost(args[1:])
	case "hosts", "devices":
		return runHost([]string{"list"})
	case "exec":
		return runExec(args[1:])
	case "terminal":
		return runTerminal(args[1:])
	case "file":
		return runFile(args[1:])
	case "android":
		return runAndroid(args[1:])
	case "doctor":
		return runDoctor()
	case "setup":
		return runSetup(args[1:])
	default:
		if len(args) == 1 && !strings.HasPrefix(args[0], "-") {
			return runTerminal(args)
		}
		return fmt.Errorf("unknown command %q (run a2h help)", args[0])
	}
}

func runSetup(args []string) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		fmt.Println("Usage: a2h setup [all|codex|claude|qoder ...]")
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("find home directory: %w", err)
	}
	executable, err := setupExecutable()
	if err != nil {
		return err
	}
	installer := integration.Installer{
		Home:       home,
		Executable: executable,
		Skill:      agent2host.AgentSkill(),
	}
	results, setupErr := installer.Setup(context.Background(), args)
	for _, result := range results {
		if result.Skipped != "" {
			fmt.Printf("%-7s skipped: %s\n", result.Agent, result.Skipped)
			continue
		}
		if result.Failed != "" {
			fmt.Printf("%-7s failed: %s\n", result.Agent, result.Failed)
			continue
		}
		fmt.Printf("%-7s connected: MCP uses %s; Skill installed at %s\n", result.Agent, executable, result.SkillPath)
	}
	return setupErr
}

func setupExecutable() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find Agent2Host executable: %w", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", fmt.Errorf("resolve Agent2Host executable: %w", err)
	}
	if strings.Contains(executable, string(filepath.Separator)+"go-build") {
		return "", errors.New("setup cannot run through go run; install or build a2h first")
	}
	return executable, nil
}

func runDaemon(args []string) error {
	if len(args) == 0 {
		return errors.New("daemon requires run, start, status, or stop")
	}
	switch args[0] {
	case "run":
		configPath, controlDir, socket, err := paths()
		if err != nil {
			return err
		}
		service, err := app.New(configPath, controlDir)
		if err != nil {
			return err
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return daemon.NewServer(service, socket).Serve(ctx)
	case "start":
		client, err := newClient(true)
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Call(daemon.MethodPing, struct{}{}, &result); err != nil {
			return err
		}
		fmt.Printf("Agent2Host daemon is running (pid %.0f)\n", result["pid"])
		return nil
	case "status":
		client, err := newClient(false)
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Call(daemon.MethodPing, struct{}{}, &result); err != nil {
			return errors.New("Agent2Host daemon is not running")
		}
		fmt.Printf("Agent2Host daemon is running (pid %.0f)\n", result["pid"])
		return nil
	case "stop":
		client, err := newClient(false)
		if err != nil {
			return err
		}
		if err := client.Call(daemon.MethodShutdown, struct{}{}, nil); err != nil {
			return err
		}
		fmt.Println("Agent2Host daemon stopped")
		return nil
	default:
		return fmt.Errorf("unknown daemon command %q", args[0])
	}
}

func runHost(args []string) error {
	if len(args) == 0 {
		return errors.New("host requires list, add, or remove")
	}
	client, err := newClient(true)
	if err != nil {
		return err
	}
	switch args[0] {
	case "list", "ls":
		var devices []model.Host
		if err := client.Call(daemon.MethodHostList, struct{}{}, &devices); err != nil {
			return err
		}
		writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(writer, "NAME\tSSH TARGET\tROOT\tSHELL")
		for _, device := range devices {
			target := device.Target
			if device.IsLocal() {
				target = "(this machine)"
			}
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", device.Name, target, device.Root, device.Shell)
		}
		return writer.Flush()
	case "add":
		flags := flag.NewFlagSet("host add", flag.ContinueOnError)
		root := flags.String("root", "", "default working directory")
		shell := flags.String("shell", "", "remote shell path")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 2 {
			return errors.New("usage: a2h host add [--root PATH] [--shell PATH] NAME SSH_TARGET")
		}
		device := model.Host{Name: flags.Arg(0), Target: flags.Arg(1), Root: *root, Shell: *shell}
		if err := client.Call(daemon.MethodHostAdd, device, nil); err != nil {
			return err
		}
		fmt.Printf("saved host %s -> %s\n", device.Name, device.Target)
		return nil
	case "remove", "rm":
		if len(args) != 2 {
			return errors.New("usage: a2h host remove NAME")
		}
		if err := client.Call(daemon.MethodHostRemove, daemon.NameParam{Name: args[1]}, nil); err != nil {
			return err
		}
		fmt.Printf("removed host %s\n", args[1])
		return nil
	default:
		return fmt.Errorf("unknown host command %q", args[0])
	}
}

func runExec(args []string) error {
	flags := flag.NewFlagSet("exec", flag.ContinueOnError)
	cwd := flags.String("cwd", "", "working directory")
	timeout := flags.Duration("timeout", 30*time.Second, "command timeout")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() < 2 {
		return errors.New("usage: a2h exec [--cwd PATH] [--timeout 30s] HOST COMMAND")
	}
	client, err := newClient(true)
	if err != nil {
		return err
	}
	request := model.ExecRequest{Host: flags.Arg(0), Command: strings.Join(flags.Args()[1:], " "), Cwd: *cwd, TimeoutMS: int(timeout.Milliseconds())}
	var result model.ExecResult
	if err := client.Call(daemon.MethodExec, request, &result); err != nil {
		return err
	}
	fmt.Print(result.Output)
	if result.OutputTruncated {
		fmt.Fprintln(os.Stderr, "a2h: command output truncated at 16 MiB")
	}
	if result.TimedOut {
		return fmt.Errorf("command timed out after %s", *timeout)
	}
	if result.ExitCode != 0 {
		return exitStatusError{code: result.ExitCode}
	}
	return nil
}

func runTerminal(args []string) error {
	flags := flag.NewFlagSet("terminal", flag.ContinueOnError)
	cwd := flags.String("cwd", "", "working directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: a2h terminal [--cwd PATH] HOST")
	}
	client, err := newClient(true)
	if err != nil {
		return err
	}
	cols, rows := 120, 40
	if term.IsTerminal(int(os.Stdin.Fd())) {
		if width, height, sizeErr := term.GetSize(int(os.Stdin.Fd())); sizeErr == nil {
			cols, rows = width, height
		}
	}
	var session model.SessionInfo
	request := model.OpenSessionRequest{Host: flags.Arg(0), Cwd: *cwd, Cols: uint16(cols), Rows: uint16(rows)}
	if err := client.Call(daemon.MethodSessionOpen, request, &session); err != nil {
		return err
	}
	defer client.Call(daemon.MethodSessionClose, daemon.SessionIDParam{SessionID: session.ID}, nil)
	return attachTerminal(client, session.ID)
}

func attachTerminal(client *daemon.Client, sessionID string) error {
	fd := int(os.Stdin.Fd())
	var oldState *term.State
	if term.IsTerminal(fd) {
		state, err := term.MakeRaw(fd)
		if err != nil {
			return err
		}
		oldState = state
		defer term.Restore(fd, oldState)
	}

	done := make(chan error, 1)
	go func() {
		cursor := int64(0)
		for {
			var result model.SessionReadResult
			err := client.Call(daemon.MethodSessionRead, daemon.SessionReadParam{SessionID: sessionID, Cursor: cursor, WaitMS: 1000}, &result)
			if err != nil {
				done <- err
				return
			}
			cursor = result.Cursor
			if result.Data != "" {
				_, _ = io.WriteString(os.Stdout, result.Data)
			}
			if result.EOF {
				done <- nil
				return
			}
		}
	}()

	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	go func() {
		for range winch {
			cols, rows, err := term.GetSize(fd)
			if err == nil {
				_ = client.Call(daemon.MethodSessionResize, daemon.SessionResizeParam{SessionID: sessionID, Cols: uint16(cols), Rows: uint16(rows)}, nil)
			}
		}
	}()

	go func() {
		buffer := make([]byte, 32<<10)
		for {
			n, err := os.Stdin.Read(buffer)
			if n > 0 {
				writeErr := client.Call(daemon.MethodSessionWrite, daemon.SessionWriteParam{SessionID: sessionID, Data: string(buffer[:n])}, nil)
				if writeErr != nil {
					done <- writeErr
					return
				}
			}
			if err != nil {
				done <- err
				return
			}
		}
	}()
	return <-done
}

func runFile(args []string) error {
	if len(args) < 3 {
		return errors.New("usage: a2h file read|write HOST PATH")
	}
	client, err := newClient(true)
	if err != nil {
		return err
	}
	switch args[0] {
	case "read":
		var result model.FileReadResult
		if err := client.Call(daemon.MethodFileRead, model.FileReadRequest{Host: args[1], Path: args[2]}, &result); err != nil {
			return err
		}
		fmt.Print(result.Content)
		if result.Truncated {
			return errors.New("file output was truncated")
		}
		return nil
	case "write":
		flags := flag.NewFlagSet("file write", flag.ContinueOnError)
		parents := flags.Bool("parents", false, "create parent directories")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 2 {
			return errors.New("usage: a2h file write [--parents] HOST PATH < CONTENT")
		}
		content, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		request := model.FileWriteRequest{Host: flags.Arg(0), Path: flags.Arg(1), Content: string(content), CreateParent: *parents}
		return client.Call(daemon.MethodFileWrite, request, nil)
	default:
		return fmt.Errorf("unknown file command %q", args[0])
	}
}

func runAndroid(args []string) error {
	if len(args) == 0 {
		return errors.New("android requires list, exec, shell, push, pull, install, logcat, or screenshot")
	}
	client, err := newClient(true)
	if err != nil {
		return err
	}
	switch args[0] {
	case "list", "ls":
		flags := flag.NewFlagSet("android list", flag.ContinueOnError)
		timeout := flags.Duration("timeout", 10*time.Second, "discovery timeout")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 1 {
			return errors.New("usage: a2h android list [--timeout 10s] HOST")
		}
		var targets []model.AndroidTarget
		request := model.AndroidListRequest{Host: flags.Arg(0), TimeoutMS: int(timeout.Milliseconds())}
		if err := client.Call(daemon.MethodAndroidList, request, &targets); err != nil {
			return err
		}
		writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(writer, "SERIAL\tSTATE\tMODEL\tPRODUCT\tTRANSPORT")
		for _, target := range targets {
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", target.Serial, target.State, target.Model, target.Product, target.TransportID)
		}
		return writer.Flush()
	case "exec":
		flags := flag.NewFlagSet("android exec", flag.ContinueOnError)
		timeout := flags.Duration("timeout", 30*time.Second, "command timeout")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() < 3 {
			return errors.New("usage: a2h android exec [--timeout 30s] HOST SERIAL COMMAND")
		}
		request := model.AndroidExecRequest{
			Host: flags.Arg(0), Serial: flags.Arg(1), Command: strings.Join(flags.Args()[2:], " "), TimeoutMS: int(timeout.Milliseconds()),
		}
		var result model.ExecResult
		if err := client.Call(daemon.MethodAndroidExec, request, &result); err != nil {
			return err
		}
		return printExecResult(result, *timeout)
	case "shell":
		flags := flag.NewFlagSet("android shell", flag.ContinueOnError)
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 2 {
			return errors.New("usage: a2h android shell HOST SERIAL")
		}
		cols, rows := terminalSize()
		request := model.AndroidOpenSessionRequest{Host: flags.Arg(0), Serial: flags.Arg(1), Cols: uint16(cols), Rows: uint16(rows)}
		var session model.SessionInfo
		if err := client.Call(daemon.MethodAndroidSessionOpen, request, &session); err != nil {
			return err
		}
		defer client.Call(daemon.MethodSessionClose, daemon.SessionIDParam{SessionID: session.ID}, nil)
		return attachTerminal(client, session.ID)
	case "push":
		flags := flag.NewFlagSet("android push", flag.ContinueOnError)
		timeout := flags.Duration("timeout", 5*time.Minute, "transfer timeout")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 4 {
			return errors.New("usage: a2h android push [--timeout 5m] HOST SERIAL LOCAL_PATH ANDROID_PATH")
		}
		localPath, err := filepath.Abs(flags.Arg(2))
		if err != nil {
			return err
		}
		request := model.AndroidPushRequest{
			Host: flags.Arg(0), Serial: flags.Arg(1), LocalPath: localPath, AndroidPath: flags.Arg(3), TimeoutMS: int(timeout.Milliseconds()),
		}
		var result model.AndroidTransferResult
		if err := client.Call(daemon.MethodAndroidPush, request, &result); err != nil {
			return err
		}
		return printTransferResult(result, *timeout)
	case "pull":
		flags := flag.NewFlagSet("android pull", flag.ContinueOnError)
		timeout := flags.Duration("timeout", 5*time.Minute, "transfer timeout")
		parents := flags.Bool("parents", false, "create local parent directories")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 4 {
			return errors.New("usage: a2h android pull [--parents] [--timeout 5m] HOST SERIAL ANDROID_PATH LOCAL_PATH")
		}
		localPath, err := filepath.Abs(flags.Arg(3))
		if err != nil {
			return err
		}
		request := model.AndroidPullRequest{
			Host: flags.Arg(0), Serial: flags.Arg(1), AndroidPath: flags.Arg(2), LocalPath: localPath, CreateParent: *parents, TimeoutMS: int(timeout.Milliseconds()),
		}
		var result model.AndroidTransferResult
		if err := client.Call(daemon.MethodAndroidPull, request, &result); err != nil {
			return err
		}
		return printTransferResult(result, *timeout)
	case "install":
		flags := flag.NewFlagSet("android install", flag.ContinueOnError)
		timeout := flags.Duration("timeout", 5*time.Minute, "installation timeout")
		replace := flags.Bool("replace", true, "replace an installed application")
		grantAll := flags.Bool("grant-all", false, "grant requested runtime permissions")
		testOnly := flags.Bool("test-only", false, "allow a test-only APK")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 3 {
			return errors.New("usage: a2h android install [--replace=false] [--grant-all] [--test-only] HOST SERIAL APK")
		}
		localPath, err := filepath.Abs(flags.Arg(2))
		if err != nil {
			return err
		}
		request := model.AndroidInstallRequest{
			Host: flags.Arg(0), Serial: flags.Arg(1), LocalPath: localPath, Replace: *replace, GrantAll: *grantAll, TestOnly: *testOnly, TimeoutMS: int(timeout.Milliseconds()),
		}
		var result model.AndroidTransferResult
		if err := client.Call(daemon.MethodAndroidInstall, request, &result); err != nil {
			return err
		}
		return printTransferResult(result, *timeout)
	case "logcat":
		flags := flag.NewFlagSet("android logcat", flag.ContinueOnError)
		timeout := flags.Duration("timeout", 30*time.Second, "log timeout")
		lines := flags.Int("lines", 200, "recent log lines")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() < 2 {
			return errors.New("usage: a2h android logcat [--lines 200] [--timeout 30s] HOST SERIAL [FILTER ...]")
		}
		request := model.AndroidLogcatRequest{
			Host: flags.Arg(0), Serial: flags.Arg(1), Lines: *lines, Filters: flags.Args()[2:], TimeoutMS: int(timeout.Milliseconds()),
		}
		var result model.ExecResult
		if err := client.Call(daemon.MethodAndroidLogcat, request, &result); err != nil {
			return err
		}
		return printExecResult(result, *timeout)
	case "screenshot":
		flags := flag.NewFlagSet("android screenshot", flag.ContinueOnError)
		timeout := flags.Duration("timeout", 30*time.Second, "capture timeout")
		parents := flags.Bool("parents", false, "create local parent directories")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 3 {
			return errors.New("usage: a2h android screenshot [--parents] [--timeout 30s] HOST SERIAL LOCAL_PATH")
		}
		localPath, err := filepath.Abs(flags.Arg(2))
		if err != nil {
			return err
		}
		request := model.AndroidScreenshotRequest{
			Host: flags.Arg(0), Serial: flags.Arg(1), LocalPath: localPath, CreateParent: *parents, TimeoutMS: int(timeout.Milliseconds()),
		}
		var result model.AndroidTransferResult
		if err := client.Call(daemon.MethodAndroidScreenshot, request, &result); err != nil {
			return err
		}
		return printTransferResult(result, *timeout)
	default:
		return fmt.Errorf("unknown android command %q", args[0])
	}
}

func terminalSize() (int, int) {
	cols, rows := 120, 40
	if term.IsTerminal(int(os.Stdin.Fd())) {
		if width, height, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
			cols, rows = width, height
		}
	}
	return cols, rows
}

func printExecResult(result model.ExecResult, timeout time.Duration) error {
	fmt.Print(result.Output)
	if result.OutputTruncated {
		fmt.Fprintln(os.Stderr, "a2h: command output truncated at 16 MiB")
	}
	if result.TimedOut {
		return fmt.Errorf("command timed out after %s", timeout)
	}
	if result.ExitCode != 0 {
		return exitStatusError{code: result.ExitCode}
	}
	return nil
}

func printTransferResult(result model.AndroidTransferResult, timeout time.Duration) error {
	if result.Output != "" {
		fmt.Fprint(os.Stderr, result.Output)
	}
	if result.TimedOut {
		return fmt.Errorf("operation timed out after %s", timeout)
	}
	if result.ExitCode != 0 {
		return exitStatusError{code: result.ExitCode}
	}
	if result.LocalPath != "" {
		fmt.Printf("saved %s (%d bytes)\n", result.LocalPath, result.Bytes)
	}
	return nil
}

func runDoctor() error {
	sshPath, sshErr := exec.LookPath("ssh")
	adbPath, adbErr := exec.LookPath("adb")
	configPath, controlDir, socket, pathErr := paths()
	status := "stopped"
	if pathErr == nil {
		client := &daemon.Client{Socket: socket}
		if err := client.Call(daemon.MethodPing, struct{}{}, nil); err == nil {
			status = "running"
		}
	}
	result := map[string]any{
		"version": buildinfo.Version, "ssh": sshPath, "local_adb": adbPath, "local_adb_available": adbErr == nil, "config": configPath, "control_dir": controlDir, "socket": socket, "daemon": status,
	}
	if sshErr != nil {
		result["ssh_error"] = sshErr.Error()
	}
	if pathErr != nil {
		result["path_error"] = pathErr.Error()
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
	if sshErr != nil || pathErr != nil {
		return errors.New("doctor found configuration errors")
	}
	return nil
}

func newClient(autoStart bool) (*daemon.Client, error) {
	socket, err := daemon.DefaultSocket()
	if err != nil {
		return nil, err
	}
	return &daemon.Client{Socket: socket, AutoStart: autoStart}, nil
}

func paths() (string, string, string, error) {
	configPath, err := config.DefaultPath()
	if err != nil {
		return "", "", "", err
	}
	controlDir, err := sshx.DefaultControlDir()
	if err != nil {
		return "", "", "", err
	}
	socket, err := daemon.DefaultSocket()
	if err != nil {
		return "", "", "", err
	}
	return configPath, controlDir, socket, nil
}

type exitStatusError struct {
	code int
}

func (e exitStatusError) Error() string {
	return "command exited with status " + strconv.Itoa(e.code)
}

func usage() {
	fmt.Print(`Agent2Host gives coding agents persistent native shells across local and SSH hosts.

Usage:
  a2h HOST
  a2h host add [--root PATH] [--shell PATH] NAME SSH_TARGET
  a2h host list
  a2h exec [--cwd PATH] [--timeout 30s] HOST COMMAND
  a2h terminal [--cwd PATH] HOST
  a2h file read HOST PATH
  a2h file write [--parents] HOST PATH < CONTENT
  a2h android list HOST
  a2h android shell HOST SERIAL
  a2h android exec HOST SERIAL COMMAND
  a2h android push HOST SERIAL LOCAL_PATH ANDROID_PATH
  a2h android pull HOST SERIAL ANDROID_PATH LOCAL_PATH
  a2h android install HOST SERIAL APK
  a2h android logcat HOST SERIAL [FILTER ...]
  a2h android screenshot HOST SERIAL LOCAL_PATH
  a2h daemon start|status|stop|run
  a2h mcp
  a2h setup [all|codex|claude|qoder ...]
  a2h doctor
`)
}
