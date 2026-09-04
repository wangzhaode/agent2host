package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type Agent string

const (
	Codex  Agent = "codex"
	Claude Agent = "claude"
	Qoder  Agent = "qoder"
)

var supportedAgents = []Agent{Codex, Claude, Qoder}

type Result struct {
	Agent      Agent
	ClientPath string
	SkillPath  string
	Skipped    string
	Failed     string
}

type Installer struct {
	Home       string
	Executable string
	Skill      []byte
	LookPath   func(string) (string, error)
	Run        func(context.Context, string, ...string) ([]byte, error)
	Getenv     func(string) string
}

func (i Installer) Setup(ctx context.Context, names []string) ([]Result, error) {
	agents, detectOnly, err := parseAgents(names)
	if err != nil {
		return nil, err
	}
	if i.Home == "" {
		return nil, errors.New("home directory is required")
	}
	if !filepath.IsAbs(i.Executable) {
		return nil, errors.New("Agent2Host executable path must be absolute")
	}
	if len(i.Skill) == 0 {
		return nil, errors.New("bundled Skill is empty")
	}
	if i.LookPath == nil {
		i.LookPath = exec.LookPath
	}
	if i.Run == nil {
		i.Run = runCommand
	}
	if i.Getenv == nil {
		i.Getenv = os.Getenv
	}

	results := make([]Result, 0, len(agents))
	var setupErrors []error
	installed := 0
	for _, agent := range agents {
		result := Result{Agent: agent}
		clientPath, err := i.findClient(agent)
		if err != nil {
			result.Skipped = err.Error()
			results = append(results, result)
			if !detectOnly {
				setupErrors = append(setupErrors, fmt.Errorf("%s: %w", agent, err))
			}
			continue
		}
		result.ClientPath = clientPath
		result.SkillPath = i.skillPath(agent)
		if err := installSkill(result.SkillPath, i.Skill); err != nil {
			result.Failed = err.Error()
			setupErrors = append(setupErrors, fmt.Errorf("%s Skill: %w", agent, err))
			results = append(results, result)
			continue
		}
		if err := i.registerMCP(ctx, agent, clientPath); err != nil {
			result.Failed = err.Error()
			setupErrors = append(setupErrors, fmt.Errorf("%s MCP: %w", agent, err))
			results = append(results, result)
			continue
		}
		installed++
		results = append(results, result)
	}
	if detectOnly && installed == 0 {
		setupErrors = append(setupErrors, errors.New("none of codex, claude, or qodercli are installed"))
	}
	return results, errors.Join(setupErrors...)
}

func parseAgents(names []string) ([]Agent, bool, error) {
	if len(names) == 0 || (len(names) == 1 && strings.EqualFold(names[0], "all")) {
		return append([]Agent(nil), supportedAgents...), true, nil
	}

	selected := make(map[Agent]bool)
	for _, name := range names {
		var agent Agent
		switch strings.ToLower(name) {
		case "codex":
			agent = Codex
		case "claude", "claude-code":
			agent = Claude
		case "qoder", "qodercli":
			agent = Qoder
		case "all":
			return nil, false, errors.New("all cannot be combined with named agents")
		default:
			return nil, false, fmt.Errorf("unsupported agent %q (choose codex, claude, qoder, or all)", name)
		}
		selected[agent] = true
	}

	agents := make([]Agent, 0, len(selected))
	for agent := range selected {
		agents = append(agents, agent)
	}
	sort.Slice(agents, func(a, b int) bool { return agents[a] < agents[b] })
	return agents, false, nil
}

func (i Installer) findClient(agent Agent) (string, error) {
	names := []string{string(agent)}
	if agent == Qoder {
		names = []string{"qodercli", "qoder"}
	}
	for _, name := range names {
		path, err := i.LookPath(name)
		if err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("client CLI not found (looked for %s)", strings.Join(names, ", "))
}

func (i Installer) skillPath(agent Agent) string {
	base := ""
	switch agent {
	case Codex:
		base = i.Getenv("CODEX_HOME")
		if base == "" {
			base = filepath.Join(i.Home, ".codex")
		}
	case Claude:
		base = i.Getenv("CLAUDE_CONFIG_DIR")
		if base == "" {
			base = filepath.Join(i.Home, ".claude")
		}
	case Qoder:
		base = filepath.Join(i.Home, ".qoder")
	}
	return filepath.Join(base, "skills", "agent2host")
}

func installSkill(destination string, content []byte) error {
	info, err := os.Lstat(destination)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(destination); err != nil {
			return fmt.Errorf("remove old Skill symlink: %w", err)
		}
	} else if err == nil && !info.IsDir() {
		return fmt.Errorf("%s exists and is not a directory", destination)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect Skill directory: %w", err)
	}

	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("create Skill directory: %w", err)
	}
	tmp, err := os.CreateTemp(destination, ".SKILL-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary Skill: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return fmt.Errorf("set Skill permissions: %w", err)
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write Skill: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close Skill: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(destination, "SKILL.md")); err != nil {
		return fmt.Errorf("replace Skill: %w", err)
	}
	return nil
}

func (i Installer) registerMCP(ctx context.Context, agent Agent, clientPath string) error {
	removeArgs := []string{"mcp", "remove", "agent2host"}
	addArgs := []string{"mcp", "add", "agent2host", "--", i.Executable, "mcp"}
	if agent == Claude || agent == Qoder {
		removeArgs = []string{"mcp", "remove", "--scope", "user", "agent2host"}
		addArgs = []string{"mcp", "add", "--scope", "user", "agent2host", "--", i.Executable, "mcp"}
	}
	_, _ = i.Run(ctx, clientPath, removeArgs...)
	output, err := i.Run(ctx, clientPath, addArgs...)
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, detail)
	}
	return nil
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
