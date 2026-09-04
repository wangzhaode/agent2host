package integration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type commandCall struct {
	name string
	args []string
}

func TestSetupAllDetectedAgents(t *testing.T) {
	home := t.TempDir()
	clients := map[string]string{
		"codex":    "/tools/codex",
		"qodercli": "/tools/qodercli",
	}
	var calls []commandCall
	installer := Installer{
		Home:       home,
		Executable: "/usr/local/bin/a2h",
		Skill:      []byte("skill body\n"),
		LookPath: func(name string) (string, error) {
			if path := clients[name]; path != "" {
				return path, nil
			}
			return "", errors.New("missing")
		},
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			calls = append(calls, commandCall{name: name, args: append([]string(nil), args...)})
			return nil, nil
		},
		Getenv: func(string) string { return "" },
	}

	results, err := installer.Setup(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("results = %#v", results)
	}
	if results[1].Agent != Claude || results[1].Skipped == "" {
		t.Fatalf("Claude result = %#v", results[1])
	}
	for _, path := range []string{
		filepath.Join(home, ".codex", "skills", "agent2host", "SKILL.md"),
		filepath.Join(home, ".qoder", "skills", "agent2host", "SKILL.md"),
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "skill body\n" {
			t.Fatalf("%s = %q", path, content)
		}
	}

	wantCalls := []commandCall{
		{name: "/tools/codex", args: []string{"mcp", "remove", "agent2host"}},
		{name: "/tools/codex", args: []string{"mcp", "add", "agent2host", "--", "/usr/local/bin/a2h", "mcp"}},
		{name: "/tools/qodercli", args: []string{"mcp", "remove", "--scope", "user", "agent2host"}},
		{name: "/tools/qodercli", args: []string{"mcp", "add", "--scope", "user", "agent2host", "--", "/usr/local/bin/a2h", "mcp"}},
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
	}
}

func TestSetupExplicitMissingAgentFails(t *testing.T) {
	installer := Installer{
		Home:       t.TempDir(),
		Executable: "/usr/local/bin/a2h",
		Skill:      []byte("skill"),
		LookPath: func(string) (string, error) {
			return "", errors.New("missing")
		},
	}
	results, err := installer.Setup(context.Background(), []string{"claude"})
	if err == nil || !strings.Contains(err.Error(), "claude") {
		t.Fatalf("err = %v", err)
	}
	if len(results) != 1 || results[0].Skipped == "" {
		t.Fatalf("results = %#v", results)
	}
}

func TestSetupReportsMCPRegistrationFailure(t *testing.T) {
	installer := Installer{
		Home:       t.TempDir(),
		Executable: "/usr/local/bin/a2h",
		Skill:      []byte("skill"),
		LookPath: func(name string) (string, error) {
			return "/tools/" + name, nil
		},
		Run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if len(args) > 1 && args[1] == "add" {
				return []byte("invalid config"), errors.New("exit status 1")
			}
			return nil, nil
		},
		Getenv: func(string) string { return "" },
	}
	results, err := installer.Setup(context.Background(), []string{"codex"})
	if err == nil || !strings.Contains(err.Error(), "invalid config") {
		t.Fatalf("err = %v", err)
	}
	if len(results) != 1 || results[0].Failed == "" {
		t.Fatalf("results = %#v", results)
	}
}

func TestSetupReplacesDevelopmentSymlink(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "source-skill")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(home, ".claude", "skills", "agent2host")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, destination); err != nil {
		t.Fatal(err)
	}

	installer := Installer{
		Home:       home,
		Executable: "/usr/local/bin/a2h",
		Skill:      []byte("new\n"),
		LookPath: func(name string) (string, error) {
			return "/tools/" + name, nil
		},
		Run:    func(context.Context, string, ...string) ([]byte, error) { return nil, nil },
		Getenv: func(string) string { return "" },
	}
	if _, err := installer.Setup(context.Background(), []string{"claude"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("Skill destination is still a symlink")
	}
	content, err := os.ReadFile(filepath.Join(destination, "SKILL.md"))
	if err != nil || string(content) != "new\n" {
		t.Fatalf("installed Skill = %q, %v", content, err)
	}
	original, err := os.ReadFile(filepath.Join(source, "SKILL.md"))
	if err != nil || string(original) != "old\n" {
		t.Fatalf("source Skill = %q, %v", original, err)
	}
}

func TestParseAgentsAliasesAndValidation(t *testing.T) {
	agents, detectOnly, err := parseAgents([]string{"qodercli", "claude-code", "codex", "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if detectOnly || !reflect.DeepEqual(agents, []Agent{Claude, Codex, Qoder}) {
		t.Fatalf("agents = %#v, detectOnly = %v", agents, detectOnly)
	}
	if _, _, err := parseAgents([]string{"all", "codex"}); err == nil {
		t.Fatal("all combined with codex succeeded")
	}
	if _, _, err := parseAgents([]string{"cursor"}); err == nil {
		t.Fatal("unsupported agent succeeded")
	}
}
