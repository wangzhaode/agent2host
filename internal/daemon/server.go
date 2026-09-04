package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wangzhaode/agent2host/internal/app"
	"github.com/wangzhaode/agent2host/internal/model"
)

type Server struct {
	service  *app.Service
	socket   string
	listener net.Listener
	done     chan struct{}
	stopOnce sync.Once
}

func NewServer(service *app.Service, socket string) *Server {
	return &Server{service: service, socket: socket, done: make(chan struct{})}
}

func (s *Server) Serve(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.socket), 0o700); err != nil {
		return fmt.Errorf("create daemon directory: %w", err)
	}
	if err := removeStaleSocket(s.socket); err != nil {
		return err
	}
	listener, err := net.Listen("unix", s.socket)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.socket, err)
	}
	s.listener = listener
	if err := os.Chmod(s.socket, 0o600); err != nil {
		listener.Close()
		return fmt.Errorf("protect daemon socket: %w", err)
	}
	defer os.Remove(s.socket)
	defer s.service.Sessions.CloseAll()

	go func() {
		select {
		case <-ctx.Done():
			s.stop()
		case <-s.done:
		}
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return nil
			case <-ctx.Done():
				return nil
			default:
				return fmt.Errorf("accept daemon connection: %w", err)
			}
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(24 * time.Hour))
	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)
	var request Request
	if err := decoder.Decode(&request); err != nil {
		_ = encoder.Encode(Response{Error: &RPCError{Code: "invalid_request", Message: err.Error()}})
		return
	}
	response := Response{ID: request.ID}
	result, err := s.dispatch(request)
	if err != nil {
		response.Error = &RPCError{Code: "operation_failed", Message: err.Error()}
	} else if result != nil {
		response.Result, err = json.Marshal(result)
		if err != nil {
			response.Error = &RPCError{Code: "encode_failed", Message: err.Error()}
		}
	}
	_ = encoder.Encode(response)
}

func (s *Server) dispatch(request Request) (any, error) {
	switch request.Method {
	case MethodPing:
		return map[string]any{"ok": true, "pid": os.Getpid()}, nil
	case MethodShutdown:
		go s.stop()
		return map[string]bool{"stopping": true}, nil
	case MethodHostList:
		return s.service.ListHosts()
	case MethodHostAdd:
		var params model.Host
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return map[string]bool{"saved": true}, s.service.AddHost(params)
	case MethodHostRemove:
		var params NameParam
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return map[string]bool{"removed": true}, s.service.RemoveHost(params.Name)
	case MethodExec:
		var params model.ExecRequest
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return s.service.Exec(params)
	case MethodSessionOpen:
		var params model.OpenSessionRequest
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return s.service.OpenSession(params)
	case MethodSessionList:
		return s.service.Sessions.List(), nil
	case MethodSessionRead:
		var params SessionReadParam
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return s.service.Sessions.Read(params.SessionID, params.Cursor, params.MaxBytes, time.Duration(params.WaitMS)*time.Millisecond)
	case MethodSessionWrite:
		var params SessionWriteParam
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		n, err := s.service.Sessions.Write(params.SessionID, params.Data)
		return map[string]int{"bytes": n}, err
	case MethodSessionExec:
		var params SessionExecParam
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return s.service.Sessions.Exec(params.SessionID, params.Command, time.Duration(params.TimeoutMS)*time.Millisecond)
	case MethodSessionResize:
		var params SessionResizeParam
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return map[string]bool{"resized": true}, s.service.Sessions.Resize(params.SessionID, params.Cols, params.Rows)
	case MethodSessionClose:
		var params SessionIDParam
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return map[string]bool{"closed": true}, s.service.Sessions.Close(params.SessionID)
	case MethodFileRead:
		var params model.FileReadRequest
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return s.service.ReadFile(params)
	case MethodFileWrite:
		var params model.FileWriteRequest
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return s.service.WriteFile(params)
	case MethodFileReplace:
		var params model.FileReplaceRequest
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return s.service.ReplaceFile(params)
	case MethodAndroidList:
		var params model.AndroidListRequest
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return s.service.ListAndroidTargets(params)
	case MethodAndroidExec:
		var params model.AndroidExecRequest
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return s.service.ExecAndroid(params)
	case MethodAndroidSessionOpen:
		var params model.AndroidOpenSessionRequest
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return s.service.OpenAndroidSession(params)
	case MethodAndroidPush:
		var params model.AndroidPushRequest
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return s.service.PushAndroid(params)
	case MethodAndroidPull:
		var params model.AndroidPullRequest
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return s.service.PullAndroid(params)
	case MethodAndroidInstall:
		var params model.AndroidInstallRequest
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return s.service.InstallAndroid(params)
	case MethodAndroidLogcat:
		var params model.AndroidLogcatRequest
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return s.service.AndroidLogcat(params)
	case MethodAndroidScreenshot:
		var params model.AndroidScreenshotRequest
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return s.service.ScreenshotAndroid(params)
	default:
		return nil, fmt.Errorf("unknown method %q", request.Method)
	}
}

func (s *Server) stop() {
	s.stopOnce.Do(func() {
		close(s.done)
		if s.listener != nil {
			_ = s.listener.Close()
		}
	})
}

func decodeParams(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return errors.New("params are required")
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}
	return nil
}

func removeStaleSocket(path string) error {
	conn, err := net.DialTimeout("unix", path, 200*time.Millisecond)
	if err == nil {
		conn.Close()
		return fmt.Errorf("Agent2Host daemon is already running at %s", path)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale daemon socket: %w", err)
	}
	return nil
}
