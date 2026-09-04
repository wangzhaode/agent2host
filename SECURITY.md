# Security Policy

## Reporting a Vulnerability

Please report security issues through GitHub's private vulnerability reporting
feature for this repository. Do not open a public issue for credentials, command
injection, authentication bypass, or unintended remote access.

Include the affected version, host platform, a minimal reproduction, and the
expected security boundary. Remove SSH keys, access tokens, device identifiers,
hostnames, and command output that could expose private infrastructure.

## Security Boundaries

Agent2Host intentionally executes commands with the authority of the local user and
their SSH credentials. It does not sandbox commands, replace the agent client's
approval policy, or weaken OpenSSH host verification.

Remote hosts require only SSH and a POSIX shell. Android support uses the adb server
already running on the selected host and always addresses a target by its serial.
