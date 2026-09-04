package daemon

import "encoding/json"

const (
	MethodPing               = "daemon.ping"
	MethodShutdown           = "daemon.shutdown"
	MethodHostList           = "host.list"
	MethodHostAdd            = "host.add"
	MethodHostRemove         = "host.remove"
	MethodExec               = "shell.exec"
	MethodSessionOpen        = "session.open"
	MethodSessionList        = "session.list"
	MethodSessionRead        = "session.read"
	MethodSessionWrite       = "session.write"
	MethodSessionExec        = "session.exec"
	MethodSessionResize      = "session.resize"
	MethodSessionClose       = "session.close"
	MethodFileRead           = "file.read"
	MethodFileWrite          = "file.write"
	MethodFileReplace        = "file.replace"
	MethodAndroidList        = "android.list"
	MethodAndroidExec        = "android.exec"
	MethodAndroidSessionOpen = "android.session.open"
	MethodAndroidPush        = "android.push"
	MethodAndroidPull        = "android.pull"
	MethodAndroidInstall     = "android.install"
	MethodAndroidLogcat      = "android.logcat"
	MethodAndroidScreenshot  = "android.screenshot"
)

type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type NameParam struct {
	Name string `json:"name" jsonschema:"configured host name"`
}

type SessionIDParam struct {
	SessionID string `json:"session_id" jsonschema:"persistent session identifier returned by session_open"`
}

type SessionReadParam struct {
	SessionID string `json:"session_id" jsonschema:"persistent session identifier"`
	Cursor    int64  `json:"cursor,omitempty" jsonschema:"previously returned cursor; use 0 for the first read"`
	MaxBytes  int    `json:"max_bytes,omitempty" jsonschema:"maximum output bytes; defaults to 65536"`
	WaitMS    int    `json:"wait_ms,omitempty" jsonschema:"long-poll duration in milliseconds"`
}

type SessionWriteParam struct {
	SessionID string `json:"session_id" jsonschema:"persistent session identifier"`
	Data      string `json:"data" jsonschema:"raw terminal input; append newline to submit a line"`
}

type SessionExecParam struct {
	SessionID string `json:"session_id" jsonschema:"persistent session identifier"`
	Command   string `json:"command" jsonschema:"shell command to run inside the session"`
	TimeoutMS int    `json:"timeout_ms,omitempty" jsonschema:"timeout in milliseconds; defaults to 30000"`
}

type SessionResizeParam struct {
	SessionID string `json:"session_id" jsonschema:"persistent session identifier"`
	Cols      uint16 `json:"cols" jsonschema:"new PTY width"`
	Rows      uint16 `json:"rows" jsonschema:"new PTY height"`
}
