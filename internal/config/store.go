package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/wangzhaode/agent2host/internal/model"
)

type fileConfig struct {
	Hosts map[string]model.Host `json:"devices"` // Keep the original on-disk key for config compatibility.
}

type Store struct {
	path string
	mu   sync.RWMutex
}

var hostNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func DefaultPath() (string, error) {
	if value := os.Getenv("AGENT2HOST_CONFIG"); value != "" {
		return value, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(dir, "agent2host", "config.json"), nil
}

func New(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) List() ([]model.Host, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}
	hosts := make([]model.Host, 0, len(cfg.Hosts)+1)
	hosts = append(hosts, model.Host{Name: model.LocalHost})
	for _, host := range cfg.Hosts {
		hosts = append(hosts, host)
	}
	sort.Slice(hosts, func(i, j int) bool { return hosts[i].Name < hosts[j].Name })
	return hosts, nil
}

func (s *Store) Get(name string) (model.Host, error) {
	if name == model.LocalHost {
		return model.Host{Name: model.LocalHost}, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg, err := s.loadUnlocked()
	if err != nil {
		return model.Host{}, err
	}
	host, ok := cfg.Hosts[name]
	if !ok {
		return model.Host{}, fmt.Errorf("host %q is not configured", name)
	}
	return host, nil
}

func (s *Store) Put(host model.Host) error {
	if host.Name == "" {
		return errors.New("host name is required")
	}
	if host.Name == model.LocalHost {
		return errors.New("local is a built-in host name")
	}
	if !hostNamePattern.MatchString(host.Name) {
		return errors.New("host name may contain only letters, numbers, dots, underscores, and hyphens")
	}
	if host.Target == "" {
		return errors.New("SSH target is required")
	}
	if strings.HasPrefix(host.Target, "-") || strings.IndexFunc(host.Target, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) >= 0 {
		return errors.New("SSH target cannot start with a hyphen or contain whitespace/control characters")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, err := s.loadUnlocked()
	if err != nil {
		return err
	}
	cfg.Hosts[host.Name] = host
	return s.saveUnlocked(cfg)
}

func (s *Store) Remove(name string) error {
	if name == model.LocalHost {
		return errors.New("the built-in local host cannot be removed")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, err := s.loadUnlocked()
	if err != nil {
		return err
	}
	if _, ok := cfg.Hosts[name]; !ok {
		return fmt.Errorf("host %q is not configured", name)
	}
	delete(cfg.Hosts, name)
	return s.saveUnlocked(cfg)
}

func (s *Store) loadUnlocked() (fileConfig, error) {
	cfg := fileConfig{Hosts: make(map[string]model.Host)}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Hosts == nil {
		cfg.Hosts = make(map[string]model.Host)
	}
	return cfg, nil
}

func (s *Store) saveUnlocked(cfg fileConfig) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("protect temporary config: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary config: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
