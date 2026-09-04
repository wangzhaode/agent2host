package android

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/wangzhaode/agent2host/internal/model"
	"github.com/wangzhaode/agent2host/internal/sshx"
)

func Command(executable string, serial string, args ...string) (string, error) {
	if executable == "" {
		executable = "adb"
	}
	if err := ValidateSerial(serial); err != nil {
		return "", err
	}
	parts := []string{sshx.ShellQuote(executable), "-s", sshx.ShellQuote(serial)}
	for _, arg := range args {
		if strings.IndexFunc(arg, unicode.IsControl) >= 0 {
			return "", errors.New("adb arguments cannot contain control characters")
		}
		parts = append(parts, sshx.ShellQuote(arg))
	}
	return strings.Join(parts, " "), nil
}

func ListCommand(executable string) string {
	if executable == "" {
		executable = "adb"
	}
	return sshx.ShellQuote(executable) + " devices -l"
}

func ValidateSerial(serial string) error {
	if serial == "" {
		return errors.New("android serial is required")
	}
	if strings.HasPrefix(serial, "-") {
		return errors.New("android serial cannot start with a hyphen")
	}
	if strings.IndexFunc(serial, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) >= 0 {
		return errors.New("android serial cannot contain whitespace or control characters")
	}
	return nil
}

func ParseDevices(host, output string) []model.AndroidTarget {
	var targets []model.AndroidTarget
	foundHeader := false
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "List of devices attached") {
			foundHeader = true
			continue
		}
		if !foundHeader || line == "" || strings.HasPrefix(line, "*") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		target := model.AndroidTarget{
			Host: host, Serial: fields[0], State: fields[1], Attributes: make(map[string]string),
		}
		start := 2
		if target.State == "no" && len(fields) > 2 && fields[2] == "permissions" {
			target.State = "no permissions"
			start = 3
		}
		for _, field := range fields[start:] {
			key, value, ok := strings.Cut(field, ":")
			if !ok || key == "" {
				continue
			}
			target.Attributes[key] = value
		}
		target.Product = target.Attributes["product"]
		target.Model = target.Attributes["model"]
		target.Codename = target.Attributes["device"]
		target.USB = target.Attributes["usb"]
		target.TransportID = target.Attributes["transport_id"]
		if len(target.Attributes) == 0 {
			target.Attributes = nil
		}
		targets = append(targets, target)
	}
	return targets
}

func RequirePath(name, path string) error {
	if path == "" {
		return fmt.Errorf("%s is required", name)
	}
	if strings.IndexFunc(path, unicode.IsControl) >= 0 {
		return fmt.Errorf("%s cannot contain control characters", name)
	}
	return nil
}
