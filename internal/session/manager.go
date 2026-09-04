package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/wangzhaode/agent2host/internal/model"
	"github.com/wangzhaode/agent2host/internal/sshx"
)

const maxBufferedOutput = 2 << 20

type Manager struct {
	transport *sshx.Transport
	mu        sync.RWMutex
	sessions  map[string]*Session
}

type Session struct {
	info   model.SessionInfo
	ptmx   *os.File
	cancel context.CancelFunc

	mu      sync.Mutex
	changed chan struct{}
	buffer  []byte
	base    int64
	closed  bool
	execMu  sync.Mutex
}

func NewManager(transport *sshx.Transport) *Manager {
	return &Manager{transport: transport, sessions: make(map[string]*Session)}
}

func (m *Manager) Open(host model.Host, request model.OpenSessionRequest) (model.SessionInfo, error) {
	cmd := m.transport.InteractiveCommand(host, request.Cwd)
	return m.open(host, request, cmd, "host", "")
}

func (m *Manager) OpenCommand(host model.Host, request model.OpenSessionRequest, command, kind, target string) (model.SessionInfo, error) {
	cmd := m.transport.InteractiveExecCommand(host, command, request.Cwd)
	return m.open(host, request, cmd, kind, target)
}

func (m *Manager) open(host model.Host, request model.OpenSessionRequest, cmd *exec.Cmd, kind, target string) (model.SessionInfo, error) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	cols, rows := request.Cols, request.Rows
	if cols == 0 {
		cols = 120
	}
	if rows == 0 {
		rows = 40
	}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		cancel()
		return model.SessionInfo{}, fmt.Errorf("start PTY: %w", err)
	}

	id, err := randomID("s_")
	if err != nil {
		ptmx.Close()
		cancel()
		return model.SessionInfo{}, err
	}
	s := &Session{
		info: model.SessionInfo{ID: id, Host: host.Name, Kind: kind, Target: target, CreatedAt: time.Now().UTC()},
		ptmx: ptmx, cancel: cancel, changed: make(chan struct{}),
	}
	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()

	go s.capture(ctx)
	go func() {
		_ = cmd.Wait()
		s.markClosed()
	}()
	return s.info, nil
}

func (m *Manager) List() []model.SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := make([]model.SessionInfo, 0, len(m.sessions))
	for _, session := range m.sessions {
		session.mu.Lock()
		info := session.info
		info.Closed = session.closed
		session.mu.Unlock()
		items = append(items, info)
	}
	return items
}

func (m *Manager) Cursor(id string) (int64, error) {
	s, err := m.get(id)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.base + int64(len(s.buffer)), nil
}

func (m *Manager) Write(id, data string) (int, error) {
	s, err := m.get(id)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return 0, errors.New("session is closed")
	}
	return s.ptmx.Write([]byte(data))
}

func (m *Manager) Resize(id string, cols, rows uint16) error {
	s, err := m.get(id)
	if err != nil {
		return err
	}
	if cols == 0 || rows == 0 {
		return errors.New("cols and rows must be positive")
	}
	return pty.Setsize(s.ptmx, &pty.Winsize{Cols: cols, Rows: rows})
}

func (m *Manager) Read(id string, cursor int64, maxBytes int, wait time.Duration) (model.SessionReadResult, error) {
	s, err := m.get(id)
	if err != nil {
		return model.SessionReadResult{}, err
	}
	if maxBytes <= 0 || maxBytes > maxBufferedOutput {
		maxBytes = 64 << 10
	}
	deadline := time.NewTimer(wait)
	if wait <= 0 {
		if !deadline.Stop() {
			<-deadline.C
		}
	}
	defer deadline.Stop()

	for {
		s.mu.Lock()
		end := s.base + int64(len(s.buffer))
		closed := s.closed
		changed := s.changed
		if cursor < s.base {
			cursor = s.base
			result := s.readLocked(cursor, maxBytes)
			result.Truncated = true
			s.mu.Unlock()
			return result, nil
		}
		if cursor < end || closed || wait <= 0 {
			result := s.readLocked(cursor, maxBytes)
			s.mu.Unlock()
			return result, nil
		}
		s.mu.Unlock()

		select {
		case <-changed:
		case <-deadline.C:
			wait = 0
		}
	}
}

func (m *Manager) Exec(id, command string, timeout time.Duration) (model.SessionExecResult, error) {
	s, err := m.get(id)
	if err != nil {
		return model.SessionExecResult{}, err
	}
	s.execMu.Lock()
	defer s.execMu.Unlock()

	token, err := randomID("sb_")
	if err != nil {
		return model.SessionExecResult{}, err
	}
	begin := "__AGENT2HOST_BEGIN_" + token + "__"
	end := "__AGENT2HOST_END_" + token + "__"
	cursor, err := m.Cursor(id)
	if err != nil {
		return model.SessionExecResult{}, err
	}
	payload := "printf '\\n" + begin + "\\n'; " + command + "; __agent2host_ec=$?; printf '\\n" + end + ":%s\\n' \"$__agent2host_ec\"\n"
	if _, err := m.Write(id, payload); err != nil {
		return model.SessionExecResult{}, err
	}

	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	deadline := time.Now().Add(timeout)
	var collected strings.Builder
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return model.SessionExecResult{Output: stripCommandEcho(collected.String(), begin), Cursor: cursor, TimedOut: true, ExitCode: -1}, nil
		}
		result, err := m.Read(id, cursor, 256<<10, min(remaining, 500*time.Millisecond))
		if err != nil {
			return model.SessionExecResult{}, err
		}
		cursor = result.Cursor
		collected.WriteString(result.Data)
		output := collected.String()
		if finish, code, found := findEnd(output, end); found {
			return model.SessionExecResult{Output: cleanExecOutput(output, begin, finish), ExitCode: code, Cursor: cursor}, nil
		}
		if result.EOF {
			return model.SessionExecResult{}, errors.New("session closed before command completed")
		}
	}
}

func (m *Manager) Close(id string) error {
	s, err := m.get(id)
	if err != nil {
		return err
	}
	s.cancel()
	_ = s.ptmx.Close()
	s.markClosed()
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
	return nil
}

func (m *Manager) CloseAll() {
	m.mu.RLock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.RUnlock()
	for _, id := range ids {
		_ = m.Close(id)
	}
}

func (m *Manager) get(id string) (*Session, error) {
	m.mu.RLock()
	s := m.sessions[id]
	m.mu.RUnlock()
	if s == nil {
		return nil, fmt.Errorf("session %q does not exist", id)
	}
	return s, nil
}

func (s *Session) capture(ctx context.Context) {
	chunk := make([]byte, 32<<10)
	for {
		n, err := s.ptmx.Read(chunk)
		if n > 0 {
			s.append(chunk[:n])
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && ctx.Err() == nil {
				s.append([]byte("\r\nAgent2Host PTY read error: " + err.Error() + "\r\n"))
			}
			s.markClosed()
			return
		}
	}
}

func (s *Session) append(data []byte) {
	s.mu.Lock()
	s.buffer = append(s.buffer, data...)
	if len(s.buffer) > maxBufferedOutput {
		drop := len(s.buffer) - maxBufferedOutput
		s.buffer = append([]byte(nil), s.buffer[drop:]...)
		s.base += int64(drop)
	}
	s.signalLocked()
	s.mu.Unlock()
}

func (s *Session) markClosed() {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		s.signalLocked()
	}
	s.mu.Unlock()
}

func (s *Session) signalLocked() {
	close(s.changed)
	s.changed = make(chan struct{})
}

func (s *Session) readLocked(cursor int64, maxBytes int) model.SessionReadResult {
	start := int(cursor - s.base)
	if start < 0 {
		start = 0
	}
	end := min(start+maxBytes, len(s.buffer))
	data := append([]byte(nil), s.buffer[start:end]...)
	return model.SessionReadResult{Data: string(data), Cursor: s.base + int64(end), EOF: s.closed && end == len(s.buffer)}
}

func randomID(prefix string) (string, error) {
	data := make([]byte, 12)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return prefix + hex.EncodeToString(data), nil
}

func findEnd(output, marker string) (int, int, bool) {
	offset := 0
	for {
		relative := strings.Index(output[offset:], marker+":")
		if relative < 0 {
			return 0, 0, false
		}
		index := offset + relative
		rest := output[index+len(marker)+1:]
		var code int
		if _, err := fmt.Sscanf(rest, "%d", &code); err == nil {
			return index, code, true
		}
		offset = index + len(marker) + 1
	}
}

func cleanExecOutput(output, begin string, finish int) string {
	start := strings.LastIndex(output[:finish], begin)
	if start >= 0 {
		output = output[start+len(begin):]
		finish -= start + len(begin)
	}
	if finish >= 0 && finish <= len(output) {
		output = output[:finish]
	}
	return strings.Trim(output, "\r\n")
}

func stripCommandEcho(output, begin string) string {
	if index := strings.LastIndex(output, begin); index >= 0 {
		return strings.Trim(output[index+len(begin):], "\r\n")
	}
	lines := strings.Split(output, "\n")
	if len(lines) > 1 {
		return strings.Trim(strings.Join(lines[1:], "\n"), "\r\n")
	}
	return strings.Trim(output, "\r\n")
}
