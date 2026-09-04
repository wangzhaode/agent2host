---
name: agent2host
description: Work in persistent native shell sessions across local and SSH hosts, and control Android targets attached to any host through adb, without installing a coding agent remotely. Use when the user asks to add or switch hosts, develop over SSH, run commands or edit files on another machine, continue work in a remote shell, interact with a terminal program, use an Android phone or emulator connected locally or to an SSH host, or coordinate coding work across Mac, Linux, and other SSH-accessible hosts.
---

# Agent2Host

Use Agent2Host as the execution boundary for host-scoped work. Never simulate a selected host by repeatedly invoking `ssh` through a generic local shell tool.

## Select a Host

1. Call `host_list` before the first host operation when the available names are unknown.
2. Use the exact host the user names. Do not maintain or infer a process-global current host.
3. Add a missing host with `host_add`; `target` may be an OpenSSH alias from the local `~/.ssh/config` or a `user@host` target.
4. Treat `local` as the built-in host for this machine.

## Reuse a Native Shell

For a development task involving several commands:

1. Check `session_list` for a live session on the intended host.
2. Reuse its `session_id`, or call `session_open` once if none is suitable.
3. Use `session_exec` for commands that complete without prompting. Changes such as `cd`, `export`, activated environments, and shell functions persist.
4. Use `session_write` and `session_read` for prompts, REPLs, debuggers, full-screen programs, and process interaction. Include `\n` when submitting a line and carry the returned `cursor` into the next read.
5. Resize interactive programs with `session_resize` when their layout is wrong.

Use `shell_exec` only for isolated commands that do not need session state.

If `session_exec` times out, inspect with `session_read`. Send `\u0003` through `session_write` to interrupt only when appropriate; the command may still be running after the client timeout.

## Work with Host Files

- Prefer `file_read` and `file_replace` for small textual edits because they avoid quoting file content through a shell.
- Use `file_write` when replacing complete UTF-8 content. Set `create_parent` only when new directories are intended.
- Relative paths resolve from the host's configured root. Use absolute paths when operating elsewhere.
- Files larger than 4 MiB or binary files should be manipulated with commands inside the persistent session.

## Work with Android Targets

Android targets are children of a host. `local` means adb runs on this machine; an SSH host means adb runs remotely and controls devices attached there.

1. Call `android_list` with the intended `host` before the first Android operation.
2. Use the exact returned `serial`. Never silently choose the first target when more than one is present.
3. Only treat state `device` as ready. For `unauthorized`, ask the user to approve USB debugging on the phone. Report `offline` without retrying destructive recovery commands.
4. Use `android_exec` for bounded commands. Use `android_session_open` once for an interactive or stateful adb shell, then reuse the returned ID with the normal `session_*` tools.
5. Use `android_push`, `android_pull`, and `android_install` for files on the Agent2Host machine. Their `local_path` must be absolute; Agent2Host streams through an SSH host automatically when needed.
6. Use `android_logcat` for bounded diagnostic snapshots and `android_screenshot` for a binary-safe PNG capture.

Do not run `adb kill-server`, change USB debugging authorization, root a target, or select another serial unless the user explicitly requests it. `adb kill-server` affects every Android target managed by that host.

## Session Lifecycle

Keep sessions alive while the user is likely to continue the same work. Close a session after an explicit request, when it is no longer needed, or before removing sensitive interactive state. The local Agent2Host daemon owns sessions, so they can survive an MCP client restart but not a daemon or machine restart.

Authentication, host aliases, jump hosts, and agent forwarding are handled by the local OpenSSH client. When first-time host verification or an authentication prompt requires interaction, open a terminal session and relay the prompt rather than changing SSH security settings. Android support uses the adb executable and adb server already installed on the selected host; it does not forward the local adb server or install Agent2Host remotely.
