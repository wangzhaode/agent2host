# Contributing to Agent2Host

Thanks for helping improve Agent2Host.

## Development Setup

Agent2Host requires Go 1.27.1 or newer on macOS or Linux.

```sh
git clone https://github.com/wangzhaode/agent2host.git
cd agent2host
go mod download
make check
```

Use `make validate` when integration validators are installed. That target includes
`make check`, validates the bundled Codex files, and invokes available Claude Code
and Qoder plugin validators.

## Changes

- Keep changes focused and follow the existing package boundaries.
- Add tests for behavior changes, especially session lifecycle, command quoting,
  binary transfer, and daemon protocol changes.
- Do not disable SSH host verification or silently select an Android target.
- Preserve the rule that remote hosts do not require Agent2Host or an agent runtime.
- Update `CHANGELOG.md` for user-visible changes.

Run these checks before opening a pull request:

```sh
make fmt
make check
make race
```

## Commits and Releases

Use clear, imperative commit messages. Releases use semantic tags such as `v0.2.0`.
Pushing a `v*` tag runs the release workflow and publishes platform archives with a
SHA-256 checksum file.
