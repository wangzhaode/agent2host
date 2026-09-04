# Agent2Host

[![CI](https://github.com/wangzhaode/agent2host/actions/workflows/ci.yml/badge.svg)](https://github.com/wangzhaode/agent2host/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Agent2Host gives humans and coding agents persistent native shell access across local and SSH hosts. The coding agent stays on your machine; remote hosts only need an SSH server and a POSIX shell.

The local daemon owns PTYs and OpenSSH control sockets. Agent sessions can keep their current directory, environment variables, background jobs, and interactive processes while the MCP client reconnects.

## Features

- A built-in `local` host plus any number of named SSH hosts.
- Persistent PTY sessions with raw read, write, resize, and command execution.
- OpenSSH `ControlMaster` reuse, including aliases, keys, jump hosts, and your existing `~/.ssh/config`.
- Atomic text reads, writes, and exact replacements on every host.
- First-class Android targets attached to either the local machine or an SSH host through that host's existing adb installation.
- A stdio MCP server for Codex and other MCP-compatible coding agents.
- A bundled Skill that teaches agents to reuse sessions instead of invoking SSH for every step.
- No Agent2Host, Codex, or other agent installation on remote hosts.

## Install

Requirements: macOS or Linux and OpenSSH. Android features additionally require
`adb` on the Host connected to the target.

Install the latest release and connect every supported coding agent detected on
this machine:

```sh
curl -fsSL https://raw.githubusercontent.com/wangzhaode/agent2host/main/install.sh | sh
```

The installer downloads the release for the current OS and architecture, verifies
its SHA-256 checksum, installs `a2h` to `~/.local/bin`, and runs `a2h setup`. To
connect only selected agents:

```sh
curl -fsSL https://raw.githubusercontent.com/wangzhaode/agent2host/main/install.sh | sh -s -- claude
curl -fsSL https://raw.githubusercontent.com/wangzhaode/agent2host/main/install.sh | sh -s -- codex qoder
```

You can install with Go instead:

```sh
go install github.com/wangzhaode/agent2host/cmd/a2h@latest
a2h setup
a2h doctor
```

`a2h setup` embeds and installs the Agent2Host Skill, then registers the local
stdio MCP server with every detected client. It currently supports:

| Client | Client command | User Skill location |
| --- | --- | --- |
| Codex | `codex` | `${CODEX_HOME:-~/.codex}/skills/agent2host` |
| Claude Code | `claude` | `${CLAUDE_CONFIG_DIR:-~/.claude}/skills/agent2host` |
| Qoder | `qodercli` or `qoder` | `~/.qoder/skills/agent2host` |

Run setup again after installing another client:

```sh
a2h setup codex
a2h setup claude
a2h setup qoder
```

Restart the configured client, then say `list my hosts`. You can add a host
entirely in chat, for example: `add host ubuntu using SSH alias ubuntu-dev with
working directory /srv/app`.

For a source checkout, Go 1.27.1 or newer is required. `make install` installs
the CLI and the legacy `./scripts/install-codex` helper remains available.

## Add Hosts

Use an existing OpenSSH alias whenever possible:

```sh
a2h host add --root /srv/app ubuntu ubuntu-dev
a2h host add --root /Users/me/work mini mac-mini
a2h host add --root /home/me/firmware riscv me@192.0.2.40
a2h hosts
```

The positional arguments are `NAME SSH_TARGET`. Agent2Host does not copy itself or an agent runtime to the remote host.

The older CLI spellings `device` and `devices` remain available as compatibility aliases. MCP uses the clearer `host_*` names: an Android phone is a Target attached to a Host.

## Open a Host

Enter a host as naturally as you would with SSH:

```sh
a2h ubuntu
a2h local
```

The explicit form is also available:

```sh
a2h terminal --cwd /srv/app ubuntu
```

Run an isolated command over the same multiplexed SSH transport:

```sh
a2h exec ubuntu 'git status --short'
a2h exec --cwd /srv/app --timeout 2m ubuntu 'go test ./...'
```

The daemon starts automatically on the first CLI or MCP operation:

```sh
a2h daemon status
a2h daemon stop
```

Stopping it closes all PTYs. To keep a remote process alive across local reboots, run `tmux` or another remote session manager on that host. This is optional remote infrastructure, not an agent installation.

## Text Files

```sh
a2h file read ubuntu internal/server.go
printf '%s\n' 'hello' | a2h file write --parents ubuntu tmp/hello.txt
```

Relative paths use the host's configured root. MCP additionally exposes `file_replace`, which requires one exact match unless `replace_all` is set.

## Android Targets

Install Android SDK Platform Tools only on the host physically or wirelessly connected to the phone. For example, if a phone is connected to `mini`, adb runs on `mini`; Agent2Host reaches it through the existing persistent SSH transport.

```sh
# Discover targets without assuming which phone is active.
a2h android list mini

# Open an interactive native Android shell.
a2h android shell mini R58M123456

# Run a bounded command and read recent logs.
a2h android exec mini R58M123456 'getprop ro.build.version.release'
a2h android logcat --lines 500 mini R58M123456 'MyApp:V' '*:S'

# Local paths are on the machine running Agent2Host. Transfers through an SSH
# host are streamed automatically and temporary remote files are removed.
a2h android push mini R58M123456 ./build/app.apk /data/local/tmp/app.apk
a2h android pull mini R58M123456 /sdcard/result.json ./result.json
a2h android install --grant-all mini R58M123456 ./build/app.apk
a2h android screenshot mini R58M123456 ./screen.png
```

Every operation requires a Host and the exact serial returned by `android list`. Agent2Host reports `device`, `offline`, and `unauthorized` states and never silently selects the first connected target.

## MCP and Plugins

This repository includes plugin metadata for Codex, Claude Code, and Qoder:

- `.mcp.json` registers the portable stdio command `a2h mcp`.
- `skills/agent2host/SKILL.md` defines the host and session workflow.
- `.codex-plugin/plugin.json`, `.claude-plugin/plugin.json`, and
  `.qoder-plugin/plugin.json` supply client-specific metadata.

The `a2h` CLI must be installed before loading a plugin directly. For normal use,
`a2h setup` is the simpler integration path because it uses the absolute binary
path and does not depend on the agent process's `PATH`.

### Aone Open Market

Publish Agent2Host to Aone as a CLI plus a Skill. The repository keeps the source
Skill compatible with the Agent Skills standard; generate the Aone package to add
the market-specific version and CLI dependency fields:

```sh
make package-aone VERSION=0.3.0
cd dist/aone-skill
a1 skill publish --dry-run
```

After reviewing the dry-run package, run `a1 skill publish` for a personal Skill or
add `--platform <code>` for a platform-owned Skill. The declared CLI dependency is
the Aone Open Platform tool named `agent2host`; its installed binary remains `a2h`.

For a generic MCP client, use:

```json
{
  "mcpServers": {
    "agent2host": {
      "command": "a2h",
      "args": ["mcp"]
    }
  }
}
```

Important MCP tools include `host_list`, `host_add`, `session_open`, `session_exec`, `session_read`, `session_write`, `shell_exec`, `file_read`, `file_write`, and `file_replace`. Android support adds `android_list`, `android_exec`, `android_session_open`, `android_push`, `android_pull`, `android_install`, `android_logcat`, and `android_screenshot`.

See [Architecture](docs/architecture.md) for the process model, transport layers,
session lifecycle, and trust boundaries.

## Data and Security

- Host definitions use the OS user configuration directory: `~/Library/Application Support/agent2host/config.json` on macOS and `${XDG_CONFIG_HOME:-~/.config}/agent2host/config.json` on Linux. Override this with `AGENT2HOST_CONFIG`.
- Runtime files use the OS user cache directory: `~/Library/Caches/agent2host` on macOS and `${XDG_CACHE_HOME:-~/.cache}/agent2host` on Linux. Override the daemon socket with `AGENT2HOST_SOCKET`.
- The Unix socket, config, logs, and SSH control sockets are user-only.
- SSH host verification and authentication remain OpenSSH's responsibility. Agent2Host does not silently disable either.
- Command and file tools have the same authority as your local SSH credentials. Agent clients should retain their normal tool approval policy.
- Android operations always use `adb -s SERIAL`. `adb kill-server` is intentionally not exposed because it disrupts every Android target on that host.

## Develop

```sh
make check
make race
make validate
```

`make check` runs formatting verification, `go vet`, tests, and a build.
`make validate` additionally runs the available Codex, Claude Code, and Qoder
integration validators. The binary is written to `bin/a2h`.

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution and release instructions.

## License

Agent2Host is available under the [MIT License](LICENSE).
