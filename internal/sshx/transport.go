package sshx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wangzhaode/agent2host/internal/model"
)

type Transport struct {
	controlDir string
}

const maxCommandOutput = 16 << 20

type Result struct {
	Output    string
	ExitCode  int
	TimedOut  bool
	Truncated bool
}

func New(controlDir string) (*Transport, error) {
	if err := os.MkdirAll(controlDir, 0o700); err != nil {
		return nil, fmt.Errorf("create SSH control directory: %w", err)
	}
	return &Transport{controlDir: controlDir}, nil
}

func DefaultControlDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("find user cache directory: %w", err)
	}
	return filepath.Join(cache, "agent2host", "ssh"), nil
}

func (t *Transport) Command(host model.Host, remoteCommand string) *exec.Cmd {
	args := t.sshArgs(host)
	if remoteCommand != "" {
		args = append(args, remoteCommand)
	}
	return exec.Command("ssh", args...)
}

func (t *Transport) Exec(ctx context.Context, host model.Host, command, cwd string, stdin io.Reader) (Result, error) {
	if host.IsLocal() {
		cmd := exec.CommandContext(ctx, localShell(host), "-lc", command)
		cmd.Dir = effectiveCwd(host, cwd)
		cmd.Stdin = stdin
		output := newCappedBuffer(maxCommandOutput)
		cmd.Stdout = &output
		cmd.Stderr = &output
		err := cmd.Run()
		code := exitCode(err)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Result{Output: output.String(), ExitCode: code, TimedOut: true, Truncated: output.truncated}, nil
		}
		return Result{Output: output.String(), ExitCode: code, Truncated: output.truncated}, commandError(err)
	}

	payload := RemotePayload(host, command, cwd)
	cmd := exec.CommandContext(ctx, "ssh", append(t.sshArgs(host), payload)...)
	cmd.Stdin = stdin
	output := newCappedBuffer(maxCommandOutput)
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	code := exitCode(err)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return Result{Output: output.String(), ExitCode: code, TimedOut: true, Truncated: output.truncated}, nil
	}
	return Result{Output: output.String(), ExitCode: code, Truncated: output.truncated}, commandError(err)
}

// Stream executes a command without buffering stdout. It is used for binary
// transfers where combining stderr with stdout would corrupt the payload.
func (t *Transport) Stream(ctx context.Context, host model.Host, command, cwd string, stdin io.Reader, stdout, stderr io.Writer) (Result, error) {
	var cmd *exec.Cmd
	if host.IsLocal() {
		cmd = exec.CommandContext(ctx, localShell(host), "-lc", command)
		cmd.Dir = effectiveCwd(host, cwd)
	} else {
		payload := RemotePayload(host, command, cwd)
		cmd = exec.CommandContext(ctx, "ssh", append(t.sshArgs(host), payload)...)
	}
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	code := exitCode(err)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return Result{ExitCode: code, TimedOut: true}, nil
	}
	return Result{ExitCode: code}, commandError(err)
}

func (t *Transport) CloseMaster(host model.Host) error {
	if host.IsLocal() {
		return nil
	}
	args := []string{"-S", t.controlPath(host), "-O", "exit", host.Target}
	if output, err := exec.Command("ssh", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("close SSH master: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func (t *Transport) InteractiveCommand(host model.Host, cwd string) *exec.Cmd {
	if host.IsLocal() {
		cmd := exec.Command(localShell(host), "-l")
		cmd.Dir = effectiveCwd(host, cwd)
		return cmd
	}
	payload := RemoteInteractivePayload(host, cwd)
	args := t.sshOptions(host)
	args = append(args, "-tt", host.Target, payload)
	return exec.Command("ssh", args...)
}

func (t *Transport) InteractiveExecCommand(host model.Host, command, cwd string) *exec.Cmd {
	if host.IsLocal() {
		cmd := exec.Command(localShell(host), "-lc", "exec "+command)
		cmd.Dir = effectiveCwd(host, cwd)
		return cmd
	}
	payload := RemotePayload(host, "exec "+command, cwd)
	args := t.sshOptions(host)
	args = append(args, "-tt", host.Target, payload)
	return exec.Command("ssh", args...)
}

func (t *Transport) sshArgs(host model.Host) []string {
	return append(t.sshOptions(host), host.Target)
}

func (t *Transport) sshOptions(host model.Host) []string {
	return []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPersist=600",
		"-o", "ControlPath=" + t.controlPath(host),
	}
}

func (t *Transport) controlPath(host model.Host) string {
	sum := sha256.Sum256([]byte(host.Target))
	return filepath.Join(t.controlDir, "cm-"+hex.EncodeToString(sum[:8]))
}

func RemotePayload(host model.Host, command, cwd string) string {
	dir := effectiveCwd(host, cwd)
	prefix := ""
	if dir != "" {
		prefix = "cd -- " + ShellQuote(dir) + " && "
	}
	shell := `"${SHELL:-/bin/sh}"`
	if host.Shell != "" {
		shell = ShellQuote(host.Shell)
	}
	return prefix + "exec " + shell + " -lc " + ShellQuote(command)
}

func RemoteInteractivePayload(host model.Host, cwd string) string {
	dir := effectiveCwd(host, cwd)
	prefix := ""
	if dir != "" {
		prefix = "cd -- " + ShellQuote(dir) + " && "
	}
	shell := `"${SHELL:-/bin/sh}"`
	if host.Shell != "" {
		shell = ShellQuote(host.Shell)
	}
	return prefix + "exec " + shell + " -l"
}

func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func localShell(host model.Host) string {
	if host.Shell != "" {
		return host.Shell
	}
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/sh"
}

func effectiveCwd(host model.Host, cwd string) string {
	if cwd != "" {
		return cwd
	}
	return host.Root
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func commandError(err error) error {
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil
	}
	return err
}

type cappedBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func newCappedBuffer(limit int) cappedBuffer {
	return cappedBuffer{limit: limit}
}

func (b *cappedBuffer) Write(data []byte) (int, error) {
	originalLength := len(data)
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		b.truncated = true
		return originalLength, nil
	}
	if len(data) > remaining {
		data = data[:remaining]
		b.truncated = true
	}
	if _, err := b.Buffer.Write(data); err != nil {
		return 0, err
	}
	return originalLength, nil
}
