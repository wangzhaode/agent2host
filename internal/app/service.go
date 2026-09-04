package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"

	"github.com/wangzhaode/agent2host/internal/config"
	"github.com/wangzhaode/agent2host/internal/model"
	"github.com/wangzhaode/agent2host/internal/session"
	"github.com/wangzhaode/agent2host/internal/sshx"
)

const defaultMaxFileBytes = 4 << 20

type Service struct {
	Config    *config.Store
	Transport *sshx.Transport
	Sessions  *session.Manager
	adb       string
}

func New(configPath, controlDir string) (*Service, error) {
	transport, err := sshx.New(controlDir)
	if err != nil {
		return nil, err
	}
	return &Service{
		Config: config.New(configPath), Transport: transport, Sessions: session.NewManager(transport), adb: "adb",
	}, nil
}

func (s *Service) ListHosts() ([]model.Host, error) {
	return s.Config.List()
}

func (s *Service) AddHost(host model.Host) error {
	return s.Config.Put(host)
}

func (s *Service) RemoveHost(name string) error {
	return s.Config.Remove(name)
}

func (s *Service) Exec(request model.ExecRequest) (model.ExecResult, error) {
	host, err := s.Config.Get(request.Host)
	if err != nil {
		return model.ExecResult{}, err
	}
	timeout := time.Duration(request.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	started := time.Now()
	result, err := s.Transport.Exec(ctx, host, request.Command, request.Cwd, nil)
	return model.ExecResult{
		Output: result.Output, ExitCode: result.ExitCode, TimedOut: result.TimedOut,
		OutputTruncated: result.Truncated, DurationMS: time.Since(started).Milliseconds(),
	}, err
}

func (s *Service) OpenSession(request model.OpenSessionRequest) (model.SessionInfo, error) {
	host, err := s.Config.Get(request.Host)
	if err != nil {
		return model.SessionInfo{}, err
	}
	return s.Sessions.Open(host, request)
}

func (s *Service) ReadFile(request model.FileReadRequest) (model.FileReadResult, error) {
	host, err := s.Config.Get(request.Host)
	if err != nil {
		return model.FileReadResult{}, err
	}
	path := resolvePath(host, request.Path)
	limit := request.MaxBytes
	if limit <= 0 || limit > defaultMaxFileBytes {
		limit = defaultMaxFileBytes
	}
	if host.IsLocal() {
		file, err := os.Open(path)
		if err != nil {
			return model.FileReadResult{}, fmt.Errorf("open file: %w", err)
		}
		defer file.Close()
		buffer, err := io.ReadAll(io.LimitReader(file, limit+1))
		if err != nil {
			return model.FileReadResult{}, fmt.Errorf("read file: %w", err)
		}
		truncated := int64(len(buffer)) > limit
		if truncated {
			buffer = buffer[:limit]
		}
		return model.FileReadResult{Content: string(buffer), Bytes: len(buffer), Truncated: truncated}, nil
	}

	command := "cat -- " + sshx.ShellQuote(path)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	run, runErr := s.Transport.Exec(ctx, host, command, "", nil)
	if runErr != nil {
		return model.FileReadResult{}, runErr
	}
	if run.TimedOut {
		return model.FileReadResult{}, errors.New("file read timed out")
	}
	if run.ExitCode != 0 {
		return model.FileReadResult{}, fmt.Errorf("remote file read failed (exit %d): %s", run.ExitCode, strings.TrimSpace(run.Output))
	}
	content := []byte(run.Output)
	truncated := run.Truncated || int64(len(content)) > limit
	if truncated {
		content = content[:limit]
	}
	return model.FileReadResult{Content: string(content), Bytes: len(content), Truncated: truncated}, nil
}

func (s *Service) WriteFile(request model.FileWriteRequest) (model.FileWriteResult, error) {
	host, err := s.Config.Get(request.Host)
	if err != nil {
		return model.FileWriteResult{}, err
	}
	path := resolvePath(host, request.Path)
	if host.IsLocal() {
		if err := atomicWriteLocal(path, []byte(request.Content), request.CreateParent); err != nil {
			return model.FileWriteResult{}, err
		}
		return model.FileWriteResult{Bytes: len(request.Content)}, nil
	}
	if err := s.atomicWriteRemote(host, path, request.Content, request.CreateParent); err != nil {
		return model.FileWriteResult{}, err
	}
	return model.FileWriteResult{Bytes: len(request.Content)}, nil
}

func (s *Service) ReplaceFile(request model.FileReplaceRequest) (model.FileReplaceResult, error) {
	if request.OldText == "" {
		return model.FileReplaceResult{}, errors.New("old_text cannot be empty")
	}
	read, err := s.ReadFile(model.FileReadRequest{Host: request.Host, Path: request.Path})
	if err != nil {
		return model.FileReplaceResult{}, err
	}
	if read.Truncated {
		return model.FileReplaceResult{}, errors.New("file is larger than the 4 MiB editable limit")
	}
	count := strings.Count(read.Content, request.OldText)
	if count == 0 {
		return model.FileReplaceResult{}, errors.New("old_text was not found")
	}
	if !request.ReplaceAll && count != 1 {
		return model.FileReplaceResult{}, fmt.Errorf("old_text matched %d times; provide more context or set replace_all", count)
	}
	replacements := 1
	countArg := 1
	if request.ReplaceAll {
		replacements = count
		countArg = -1
	}
	content := strings.Replace(read.Content, request.OldText, request.NewText, countArg)
	if _, err := s.WriteFile(model.FileWriteRequest{Host: request.Host, Path: request.Path, Content: content}); err != nil {
		return model.FileReplaceResult{}, err
	}
	return model.FileReplaceResult{Replacements: replacements, Bytes: len(content)}, nil
}

func (s *Service) atomicWriteRemote(host model.Host, path, content string, createParent bool) error {
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		return fmt.Errorf("generate temporary filename: %w", err)
	}
	tmp := path + ".agent2host-" + hex.EncodeToString(random) + ".tmp"
	parts := []string{"set -e"}
	if createParent {
		parts = append(parts, "mkdir -p -- "+sshx.ShellQuote(pathpkg.Dir(path)))
	}
	cleanup := "rm -f -- " + sshx.ShellQuote(tmp)
	parts = append(parts,
		"umask 077",
		"trap "+sshx.ShellQuote(cleanup)+" EXIT HUP INT TERM",
		"cat > "+sshx.ShellQuote(tmp),
		"if [ -e "+sshx.ShellQuote(path)+" ]; then mode=$(stat -c '%a' "+sshx.ShellQuote(path)+" 2>/dev/null || stat -f '%Lp' "+sshx.ShellQuote(path)+"); chmod \"$mode\" "+sshx.ShellQuote(tmp)+"; fi",
		"mv -f -- "+sshx.ShellQuote(tmp)+" "+sshx.ShellQuote(path),
		"trap - EXIT HUP INT TERM",
	)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	run, runErr := s.Transport.Exec(ctx, host, strings.Join(parts, "; "), "", strings.NewReader(content))
	if runErr != nil {
		return runErr
	}
	if run.TimedOut {
		return errors.New("file write timed out")
	}
	if run.ExitCode != 0 {
		return fmt.Errorf("remote file write failed (exit %d): %s", run.ExitCode, strings.TrimSpace(run.Output))
	}
	return nil
}

func resolvePath(host model.Host, path string) string {
	if filepath.IsAbs(path) || host.Root == "" {
		return path
	}
	if host.IsLocal() {
		return filepath.Join(host.Root, path)
	}
	return pathpkg.Join(host.Root, path)
}

func atomicWriteLocal(path string, content []byte, createParent bool) error {
	dir := filepath.Dir(path)
	if createParent {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create parent directory: %w", err)
		}
	}
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat destination: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".agent2host-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace destination: %w", err)
	}
	return nil
}
