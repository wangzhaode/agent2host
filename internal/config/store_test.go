package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wangzhaode/agent2host/internal/model"
)

func TestStoreLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	store := New(path)

	devices, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].Name != model.LocalHost {
		t.Fatalf("initial devices = %#v", devices)
	}

	want := model.Host{Name: "ubuntu", Target: "ubuntu-dev", Root: "/srv/app", Shell: "/bin/bash"}
	if err := store.Put(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get("ubuntu")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Get() = %#v, want %#v", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("config mode = %o, want 600", mode)
	}

	if err := store.Remove("ubuntu"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("ubuntu"); err == nil {
		t.Fatal("Get() succeeded after remove")
	}
}

func TestStoreRejectsReservedLocalName(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "config.json"))
	if err := store.Put(model.Host{Name: model.LocalHost, Target: "host"}); err == nil {
		t.Fatal("Put(local) succeeded")
	}
	if err := store.Remove(model.LocalHost); err == nil {
		t.Fatal("Remove(local) succeeded")
	}
}

func TestStoreRejectsUnsafeDeviceValues(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "config.json"))
	for _, device := range []model.Host{
		{Name: "has space", Target: "host"},
		{Name: "option", Target: "-oProxyCommand=bad"},
		{Name: "space", Target: "user@bad host"},
	} {
		if err := store.Put(device); err == nil {
			t.Errorf("Put(%#v) succeeded", device)
		}
	}
}
