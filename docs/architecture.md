# Architecture

Agent2Host keeps the coding agent local while giving it persistent execution
contexts on local and SSH hosts. Android devices are targets attached to a host,
not independent SSH hosts.

```text
Coding Agent
    |
    | MCP over stdio
    v
a2h MCP process
    |
    | JSON RPC over a user-only Unix socket
    v
Agent2Host daemon
    |                         |
    | local PTY               | OpenSSH + ControlMaster
    v                         v
local Host                 SSH Host
    |                         |
    | adb                     | adb
    v                         v
Android Target            Android Target
```

## Components

`cmd/a2h` provides the CLI, daemon entry point, and stdio MCP entry point.

`internal/daemon` owns the local Unix socket protocol. The daemon is the lifecycle
boundary for PTYs, session output buffers, and SSH control sockets.

`internal/session` starts PTY-backed processes and retains bounded output so MCP
clients can reconnect using cursors. A session records its Host, kind, and optional
Android serial.

`internal/sshx` is the execution transport. It runs local shells directly and SSH
commands through the user's OpenSSH client. This preserves aliases, ProxyJump,
agent forwarding, authentication, and host verification behavior.

`internal/app` coordinates configuration, transport, sessions, host files, and
Android operations. Binary Android transfers keep stdout separate from stderr and
use protected temporary files on an intermediate SSH host.

`internal/mcpserver` exposes narrow, structured tools. Host and Android selection is
explicit in every request; there is no process-global current Host or Android target.

`internal/integration` installs the embedded Skill and registers the stdio MCP
command with supported local coding-agent clients. Every client points to the same
`a2h` binary and daemon, so Host configuration and persistent sessions are shared.

## Trust Model

- The daemon socket, configuration, logs, and SSH control sockets are user-only.
- Commands have the same authority as the user running Agent2Host.
- OpenSSH remains responsible for authentication and host verification.
- Android operations always include `adb -s SERIAL`.
- Agent2Host never installs itself on SSH hosts or Android targets.
- Host-wide recovery commands such as `adb kill-server` are not exposed as
  dedicated MCP tools.

## Session Lifecycle

The CLI or MCP process starts the daemon on demand. Sessions survive MCP client
restarts but end when the daemon or local machine stops. Remote processes that must
survive a local reboot should run under a remote supervisor such as `tmux` or a
service manager.
