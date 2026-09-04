package model

import "time"

const LocalHost = "local"

type Host struct {
	Name   string `json:"name" jsonschema:"short stable host name used by tools"`
	Target string `json:"target,omitempty" jsonschema:"OpenSSH alias or user@host target; omit only for the built-in local host"`
	Root   string `json:"root,omitempty" jsonschema:"default working directory and base for relative file paths"`
	Shell  string `json:"shell,omitempty" jsonschema:"optional shell executable path; defaults to the login shell"`
}

func (h Host) IsLocal() bool {
	return h.Name == LocalHost || h.Target == ""
}

type ExecRequest struct {
	Host      string `json:"host" jsonschema:"configured host name or local"`
	Command   string `json:"command" jsonschema:"shell command to execute"`
	Cwd       string `json:"cwd,omitempty" jsonschema:"working directory override"`
	TimeoutMS int    `json:"timeout_ms,omitempty" jsonschema:"timeout in milliseconds; defaults to 30000"`
}

type ExecResult struct {
	Output          string `json:"output"`
	ExitCode        int    `json:"exit_code"`
	DurationMS      int64  `json:"duration_ms"`
	TimedOut        bool   `json:"timed_out,omitempty"`
	OutputTruncated bool   `json:"output_truncated,omitempty"`
}

type OpenSessionRequest struct {
	Host string `json:"host" jsonschema:"configured host name or local"`
	Cwd  string `json:"cwd,omitempty" jsonschema:"initial working directory override"`
	Cols uint16 `json:"cols,omitempty" jsonschema:"PTY width; defaults to 120"`
	Rows uint16 `json:"rows,omitempty" jsonschema:"PTY height; defaults to 40"`
}

type SessionInfo struct {
	ID        string    `json:"id"`
	Host      string    `json:"host"`
	Kind      string    `json:"kind,omitempty"`
	Target    string    `json:"target,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Closed    bool      `json:"closed"`
}

type SessionReadResult struct {
	Data      string `json:"data"`
	Cursor    int64  `json:"cursor"`
	Truncated bool   `json:"truncated,omitempty"`
	EOF       bool   `json:"eof,omitempty"`
}

type SessionExecResult struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
	Cursor   int64  `json:"cursor"`
	TimedOut bool   `json:"timed_out,omitempty"`
}

type FileReadRequest struct {
	Host     string `json:"host" jsonschema:"configured host name or local"`
	Path     string `json:"path" jsonschema:"absolute path or path relative to the host root"`
	MaxBytes int64  `json:"max_bytes,omitempty" jsonschema:"maximum bytes to return, capped at 4194304"`
}

type FileReadResult struct {
	Content   string `json:"content"`
	Bytes     int    `json:"bytes"`
	Truncated bool   `json:"truncated,omitempty"`
}

type FileWriteRequest struct {
	Host         string `json:"host" jsonschema:"configured host name or local"`
	Path         string `json:"path" jsonschema:"absolute path or path relative to the host root"`
	Content      string `json:"content" jsonschema:"complete UTF-8 file content"`
	CreateParent bool   `json:"create_parent,omitempty" jsonschema:"create missing parent directories"`
}

type FileWriteResult struct {
	Bytes int `json:"bytes"`
}

type FileReplaceRequest struct {
	Host       string `json:"host" jsonschema:"configured host name or local"`
	Path       string `json:"path" jsonschema:"absolute path or path relative to the host root"`
	OldText    string `json:"old_text" jsonschema:"exact text to replace; include context so the match is unique"`
	NewText    string `json:"new_text" jsonschema:"replacement text"`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema:"replace every match instead of requiring exactly one"`
}

type FileReplaceResult struct {
	Replacements int `json:"replacements"`
	Bytes        int `json:"bytes"`
}
