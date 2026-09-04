package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wangzhaode/agent2host/internal/android"
	"github.com/wangzhaode/agent2host/internal/model"
	"github.com/wangzhaode/agent2host/internal/sshx"
)

const (
	defaultAndroidTimeout  = 30 * time.Second
	defaultTransferTimeout = 5 * time.Minute
)

func (s *Service) ListAndroidTargets(request model.AndroidListRequest) ([]model.AndroidTarget, error) {
	host, err := s.Config.Get(request.Host)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout(request.TimeoutMS, 10*time.Second))
	defer cancel()
	run, err := s.Transport.Exec(ctx, host, android.ListCommand(s.adb), "", nil)
	if err != nil {
		return nil, err
	}
	if run.TimedOut {
		return nil, errors.New("adb device discovery timed out")
	}
	if run.ExitCode != 0 {
		return nil, fmt.Errorf("adb device discovery failed (exit %d): %s", run.ExitCode, strings.TrimSpace(run.Output))
	}
	return android.ParseDevices(host.Name, run.Output), nil
}

func (s *Service) ExecAndroid(request model.AndroidExecRequest) (model.ExecResult, error) {
	if strings.TrimSpace(request.Command) == "" {
		return model.ExecResult{}, errors.New("android command is required")
	}
	host, err := s.Config.Get(request.Host)
	if err != nil {
		return model.ExecResult{}, err
	}
	command, err := android.Command(s.adb, request.Serial, "shell", "sh")
	if err != nil {
		return model.ExecResult{}, err
	}
	timeout := requestTimeout(request.TimeoutMS, defaultAndroidTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	started := time.Now()
	run, err := s.Transport.Exec(ctx, host, command, "", strings.NewReader(request.Command+"\n"))
	return execResult(run, started), err
}

func (s *Service) OpenAndroidSession(request model.AndroidOpenSessionRequest) (model.SessionInfo, error) {
	host, err := s.Config.Get(request.Host)
	if err != nil {
		return model.SessionInfo{}, err
	}
	command, err := android.Command(s.adb, request.Serial, "shell")
	if err != nil {
		return model.SessionInfo{}, err
	}
	open := model.OpenSessionRequest{Host: host.Name, Cols: request.Cols, Rows: request.Rows}
	return s.Sessions.OpenCommand(host, open, command, "android", request.Serial)
}

func (s *Service) PushAndroid(request model.AndroidPushRequest) (model.AndroidTransferResult, error) {
	if err := requireAbsoluteLocalPath(request.LocalPath); err != nil {
		return model.AndroidTransferResult{}, err
	}
	if err := android.RequirePath("android_path", request.AndroidPath); err != nil {
		return model.AndroidTransferResult{}, err
	}
	host, err := s.Config.Get(request.Host)
	if err != nil {
		return model.AndroidTransferResult{}, err
	}
	source, err := os.Open(request.LocalPath)
	if err != nil {
		return model.AndroidTransferResult{}, fmt.Errorf("open local source: %w", err)
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return model.AndroidTransferResult{}, fmt.Errorf("stat local source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return model.AndroidTransferResult{}, errors.New("local_path must be a regular file")
	}
	adb, err := android.Command(s.adb, request.Serial, "push")
	if err != nil {
		return model.AndroidTransferResult{}, err
	}
	command, err := stagedCommand(adb+` "$tmp" `+sshx.ShellQuote(request.AndroidPath), "")
	if err != nil {
		return model.AndroidTransferResult{}, err
	}
	return s.runAndroidTransfer(host, command, source, info.Size(), requestTimeout(request.TimeoutMS, defaultTransferTimeout))
}

func (s *Service) PullAndroid(request model.AndroidPullRequest) (model.AndroidTransferResult, error) {
	if err := requireAbsoluteLocalPath(request.LocalPath); err != nil {
		return model.AndroidTransferResult{}, err
	}
	if err := android.RequirePath("android_path", request.AndroidPath); err != nil {
		return model.AndroidTransferResult{}, err
	}
	host, err := s.Config.Get(request.Host)
	if err != nil {
		return model.AndroidTransferResult{}, err
	}
	adb, err := android.Command(s.adb, request.Serial, "pull", request.AndroidPath)
	if err != nil {
		return model.AndroidTransferResult{}, err
	}
	command, err := stagedCommand(adb+` "$tmp" >&2; cat -- "$tmp"`, "")
	if err != nil {
		return model.AndroidTransferResult{}, err
	}
	return s.streamAndroidFile(host, command, request.LocalPath, request.CreateParent, nil, requestTimeout(request.TimeoutMS, defaultTransferTimeout))
}

func (s *Service) InstallAndroid(request model.AndroidInstallRequest) (model.AndroidTransferResult, error) {
	if err := requireAbsoluteLocalPath(request.LocalPath); err != nil {
		return model.AndroidTransferResult{}, err
	}
	host, err := s.Config.Get(request.Host)
	if err != nil {
		return model.AndroidTransferResult{}, err
	}
	source, err := os.Open(request.LocalPath)
	if err != nil {
		return model.AndroidTransferResult{}, fmt.Errorf("open APK: %w", err)
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return model.AndroidTransferResult{}, fmt.Errorf("stat APK: %w", err)
	}
	if !info.Mode().IsRegular() {
		return model.AndroidTransferResult{}, errors.New("local_path must be a regular APK file")
	}
	args := []string{"install"}
	if request.Replace {
		args = append(args, "-r")
	}
	if request.GrantAll {
		args = append(args, "-g")
	}
	if request.TestOnly {
		args = append(args, "-t")
	}
	adb, err := android.Command(s.adb, request.Serial, args...)
	if err != nil {
		return model.AndroidTransferResult{}, err
	}
	command, err := stagedCommand(adb+` "$tmp"`, ".apk")
	if err != nil {
		return model.AndroidTransferResult{}, err
	}
	return s.runAndroidTransfer(host, command, source, info.Size(), requestTimeout(request.TimeoutMS, defaultTransferTimeout))
}

func (s *Service) AndroidLogcat(request model.AndroidLogcatRequest) (model.ExecResult, error) {
	host, err := s.Config.Get(request.Host)
	if err != nil {
		return model.ExecResult{}, err
	}
	lines := request.Lines
	if lines <= 0 {
		lines = 200
	}
	if lines > 100000 {
		return model.ExecResult{}, errors.New("logcat lines cannot exceed 100000")
	}
	args := []string{"logcat", "-d", "-t", strconv.Itoa(lines)}
	args = append(args, request.Filters...)
	command, err := android.Command(s.adb, request.Serial, args...)
	if err != nil {
		return model.ExecResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout(request.TimeoutMS, defaultAndroidTimeout))
	defer cancel()
	started := time.Now()
	run, err := s.Transport.Exec(ctx, host, command, "", nil)
	return execResult(run, started), err
}

func (s *Service) ScreenshotAndroid(request model.AndroidScreenshotRequest) (model.AndroidTransferResult, error) {
	if err := requireAbsoluteLocalPath(request.LocalPath); err != nil {
		return model.AndroidTransferResult{}, err
	}
	host, err := s.Config.Get(request.Host)
	if err != nil {
		return model.AndroidTransferResult{}, err
	}
	command, err := android.Command(s.adb, request.Serial, "exec-out", "screencap", "-p")
	if err != nil {
		return model.AndroidTransferResult{}, err
	}
	return s.streamAndroidFile(host, command, request.LocalPath, request.CreateParent, nil, requestTimeout(request.TimeoutMS, defaultAndroidTimeout))
}

func (s *Service) runAndroidTransfer(host model.Host, command string, input io.Reader, bytesCount int64, timeout time.Duration) (model.AndroidTransferResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	started := time.Now()
	run, err := s.Transport.Exec(ctx, host, command, "", input)
	result := model.AndroidTransferResult{
		Output: run.Output, ExitCode: run.ExitCode, DurationMS: time.Since(started).Milliseconds(), TimedOut: run.TimedOut,
	}
	if !run.TimedOut && run.ExitCode == 0 {
		result.Bytes = bytesCount
	}
	return result, err
}

func (s *Service) streamAndroidFile(host model.Host, command, localPath string, createParent bool, input io.Reader, timeout time.Duration) (model.AndroidTransferResult, error) {
	dir := filepath.Dir(localPath)
	if createParent {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return model.AndroidTransferResult{}, fmt.Errorf("create local parent directory: %w", err)
		}
	}
	mode := os.FileMode(0o600)
	if info, err := os.Stat(localPath); err == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return model.AndroidTransferResult{}, fmt.Errorf("stat local destination: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".agent2host-android-*")
	if err != nil {
		return model.AndroidTransferResult{}, fmt.Errorf("create local temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return model.AndroidTransferResult{}, err
	}

	var stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	started := time.Now()
	run, runErr := s.Transport.Stream(ctx, host, command, "", input, tmp, &stderr)
	closeErr := tmp.Close()
	result := model.AndroidTransferResult{
		Output: stderr.String(), ExitCode: run.ExitCode, DurationMS: time.Since(started).Milliseconds(), TimedOut: run.TimedOut, LocalPath: localPath,
	}
	if runErr != nil {
		return result, runErr
	}
	if closeErr != nil {
		return result, closeErr
	}
	if run.TimedOut || run.ExitCode != 0 {
		return result, nil
	}
	info, err := os.Stat(tmpPath)
	if err != nil {
		return result, err
	}
	result.Bytes = info.Size()
	if err := os.Rename(tmpPath, localPath); err != nil {
		return result, fmt.Errorf("replace local destination: %w", err)
	}
	return result, nil
}

func stagedCommand(operation, suffix string) (string, error) {
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate Android transfer path: %w", err)
	}
	name := ".agent2host-android-" + hex.EncodeToString(random) + suffix
	return strings.Join([]string{
		"set -e",
		`tmp="${TMPDIR:-/tmp}/` + name + `"`,
		`trap 'rm -f -- "$tmp"' EXIT HUP INT TERM`,
		"umask 077",
		`cat > "$tmp"`,
		operation,
	}, "; "), nil
}

func requestTimeout(milliseconds int, fallback time.Duration) time.Duration {
	timeout := time.Duration(milliseconds) * time.Millisecond
	if timeout <= 0 {
		return fallback
	}
	return timeout
}

func requireAbsoluteLocalPath(path string) error {
	if path == "" {
		return errors.New("local_path is required")
	}
	if !filepath.IsAbs(path) {
		return errors.New("local_path must be absolute")
	}
	return nil
}

func execResult(run sshx.Result, started time.Time) model.ExecResult {
	return model.ExecResult{
		Output: run.Output, ExitCode: run.ExitCode, DurationMS: time.Since(started).Milliseconds(), TimedOut: run.TimedOut, OutputTruncated: run.Truncated,
	}
}
