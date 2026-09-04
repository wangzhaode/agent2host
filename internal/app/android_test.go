package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wangzhaode/agent2host/internal/model"
)

func TestAndroidOperationsOnLocalHost(t *testing.T) {
	root := t.TempDir()
	service, err := New(filepath.Join(root, "config.json"), filepath.Join(root, "ssh"))
	if err != nil {
		t.Fatal(err)
	}
	service.adb = writeFakeADB(t, root)

	targets, err := service.ListAndroidTargets(model.AndroidListRequest{Host: model.LocalHost})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 || targets[0].Serial != "test-serial" || targets[1].State != "unauthorized" {
		t.Fatalf("targets = %#v", targets)
	}

	executed, err := service.ExecAndroid(model.AndroidExecRequest{
		Host: model.LocalHost, Serial: "test-serial", Command: "printf android-exec",
	})
	if err != nil {
		t.Fatal(err)
	}
	if executed.ExitCode != 0 || executed.Output != "android-exec" {
		t.Fatalf("exec result = %#v", executed)
	}

	source := filepath.Join(root, "source.bin")
	payload := []byte{0x00, 0x01, 0x0a, 0xff, 'A'}
	if err := os.WriteFile(source, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	pushed, err := service.PushAndroid(model.AndroidPushRequest{
		Host: model.LocalHost, Serial: "test-serial", LocalPath: source, AndroidPath: "/data/local/tmp/source.bin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if pushed.ExitCode != 0 || pushed.Bytes != int64(len(payload)) || !strings.Contains(pushed.Output, "pushed 5 bytes") {
		t.Fatalf("push result = %#v", pushed)
	}

	pulledPath := filepath.Join(root, "nested", "pulled.bin")
	pulled, err := service.PullAndroid(model.AndroidPullRequest{
		Host: model.LocalHost, Serial: "test-serial", AndroidPath: "/sdcard/pulled.bin", LocalPath: pulledPath, CreateParent: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pulledData, err := os.ReadFile(pulledPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pulledData, payload) || pulled.Bytes != int64(len(payload)) {
		t.Fatalf("pull result = %#v, data = %v", pulled, pulledData)
	}

	installed, err := service.InstallAndroid(model.AndroidInstallRequest{
		Host: model.LocalHost, Serial: "test-serial", LocalPath: source, Replace: true, GrantAll: true, TestOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if installed.ExitCode != 0 || !strings.Contains(installed.Output, "Success") {
		t.Fatalf("install result = %#v", installed)
	}

	logs, err := service.AndroidLogcat(model.AndroidLogcatRequest{
		Host: model.LocalHost, Serial: "test-serial", Lines: 10, Filters: []string{"Demo:I", "*:S"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if logs.ExitCode != 0 || logs.Output != "demo log line\n" {
		t.Fatalf("logcat result = %#v", logs)
	}

	screenshotPath := filepath.Join(root, "captures", "screen.png")
	screenshot, err := service.ScreenshotAndroid(model.AndroidScreenshotRequest{
		Host: model.LocalHost, Serial: "test-serial", LocalPath: screenshotPath, CreateParent: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	screenshotData, err := os.ReadFile(screenshotPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(screenshotData, []byte("\x89PNG\r\n\x1a\n")) || screenshot.Bytes != 8 {
		t.Fatalf("screenshot result = %#v, data = %v", screenshot, screenshotData)
	}
}

func TestAndroidInteractiveSession(t *testing.T) {
	root := t.TempDir()
	service, err := New(filepath.Join(root, "config.json"), filepath.Join(root, "ssh"))
	if err != nil {
		t.Fatal(err)
	}
	service.adb = writeFakeADB(t, root)
	info, err := service.OpenAndroidSession(model.AndroidOpenSessionRequest{Host: model.LocalHost, Serial: "test-serial"})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Sessions.Close(info.ID)
	if info.Kind != "android" || info.Target != "test-serial" || info.Host != model.LocalHost {
		t.Fatalf("session info = %#v", info)
	}
	result, err := service.Sessions.Exec(info.ID, "printf android-session", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || !strings.Contains(result.Output, "android-session") {
		t.Fatalf("session result = %#v", result)
	}
}

func TestAndroidRequiresAbsoluteLocalPaths(t *testing.T) {
	root := t.TempDir()
	service, err := New(filepath.Join(root, "config.json"), filepath.Join(root, "ssh"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PushAndroid(model.AndroidPushRequest{
		Host: model.LocalHost, Serial: "test-serial", LocalPath: "relative.apk", AndroidPath: "/data/local/tmp/app.apk",
	}); err == nil {
		t.Fatal("relative push source succeeded")
	}
}

func writeFakeADB(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "fake adb")
	script := `#!/bin/sh
if [ "$1" = devices ]; then
  printf '%s\n' 'List of devices attached'
  printf '%s\n' 'test-serial device product:test_product model:Test_Phone device:test transport_id:1'
  printf '%s\n' 'locked-serial unauthorized usb:1-2 transport_id:2'
  exit 0
fi

if [ "$1" != -s ] || [ "$2" != test-serial ]; then
  printf '%s\n' 'bad target' >&2
  exit 2
fi

case "$3" in
  shell)
    exec /bin/sh
    ;;
  push)
    size=$(wc -c < "$4" | tr -d ' ')
    printf 'pushed %s bytes\n' "$size"
    ;;
  pull)
    printf '\000\001\012\377A' > "$5"
    printf '%s\n' 'pulled test file' >&2
    ;;
  install)
    for arg in "$@"; do last=$arg; done
    test -f "$last" || exit 3
    printf '%s\n' 'Success'
    ;;
  logcat)
    printf '%s\n' 'demo log line'
    ;;
  exec-out)
    printf '\211PNG\015\012\032\012'
    ;;
  *)
    printf 'unsupported fake adb command: %s\n' "$3" >&2
    exit 4
    ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
