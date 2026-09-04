package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type Client struct {
	Socket    string
	AutoStart bool
}

func DefaultSocket() (string, error) {
	if value := os.Getenv("AGENT2HOST_SOCKET"); value != "" {
		return value, nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("find user cache directory: %w", err)
	}
	return filepath.Join(cache, "agent2host", "daemon.sock"), nil
}

func (c *Client) Call(method string, params, result any) error {
	err := c.callOnce(method, params, result)
	if err == nil || !c.AutoStart || !isConnectionError(err) {
		return err
	}
	if startErr := c.startDaemon(); startErr != nil {
		return fmt.Errorf("start daemon after connection failure (%v): %w", err, startErr)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		if err = c.callOnce(method, params, result); err == nil {
			return nil
		}
	}
	return fmt.Errorf("daemon did not become ready: %w", err)
}

func (c *Client) callOnce(method string, params, result any) error {
	conn, err := net.DialTimeout("unix", c.Socket, 500*time.Millisecond)
	if err != nil {
		return fmt.Errorf("connect to daemon: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(11 * time.Minute))
	raw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encode params: %w", err)
	}
	request := Request{ID: fmt.Sprintf("%d", time.Now().UnixNano()), Method: method, Params: raw}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return fmt.Errorf("send daemon request: %w", err)
	}
	var response Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return fmt.Errorf("read daemon response: %w", err)
	}
	if response.Error != nil {
		return errors.New(response.Error.Message)
	}
	if result != nil && len(response.Result) > 0 {
		if err := json.Unmarshal(response.Result, result); err != nil {
			return fmt.Errorf("decode daemon response: %w", err)
		}
	}
	return nil
}

func (c *Client) startDaemon() error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate a2h executable: %w", err)
	}
	logPath := filepath.Join(filepath.Dir(c.Socket), "daemon.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open daemon log: %w", err)
	}
	defer logFile.Close()
	cmd := exec.Command(executable, "daemon", "run")
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch daemon: %w", err)
	}
	return cmd.Process.Release()
}

func isConnectionError(err error) bool {
	var netErr *net.OpError
	return errors.As(err, &netErr) || errors.Is(err, os.ErrNotExist)
}
