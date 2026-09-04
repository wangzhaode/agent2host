# Changelog

All notable changes to Agent2Host will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.0] - 2026-09-04

### Added

- Persistent PTY sessions on the local machine and named SSH hosts.
- OpenSSH connection multiplexing through the user's existing SSH configuration.
- Structured MCP tools for host commands, sessions, and atomic text file operations.
- Android target discovery, shell execution, interactive sessions, file transfer,
  APK installation, logcat snapshots, and screenshots through adb on any host.
- `a2h setup` for automatic user-level integration with Codex, Claude Code, and
  Qoder.
- An embedded Agent2Host Skill so installed binaries do not depend on a source
  checkout.
- Codex, Claude Code, and Qoder plugin metadata.
- A checksum-verifying installer for GitHub Release binaries.
- Aone Skill packaging with an `agent2host` CLI dependency.

### Changed

- Release archives now use stable OS and architecture names for latest-release
  installation URLs.
- The portable MCP configuration launches `a2h mcp` directly.

[Unreleased]: https://github.com/wangzhaode/agent2host/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/wangzhaode/agent2host/releases/tag/v0.3.0
