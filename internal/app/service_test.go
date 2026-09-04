package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wangzhaode/agent2host/internal/model"
)

func TestLocalFileRoundTripAndReplace(t *testing.T) {
	root := t.TempDir()
	service, err := New(filepath.Join(root, "config.json"), filepath.Join(root, "ssh"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "nested", "hello.txt")
	write, err := service.WriteFile(model.FileWriteRequest{
		Host: model.LocalHost, Path: path, Content: "hello world\n", CreateParent: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if write.Bytes != len("hello world\n") {
		t.Fatalf("write bytes = %d", write.Bytes)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	replaced, err := service.ReplaceFile(model.FileReplaceRequest{
		Host: model.LocalHost, Path: path, OldText: "world", NewText: "bridge",
	})
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Replacements != 1 {
		t.Fatalf("replacements = %d", replaced.Replacements)
	}
	read, err := service.ReadFile(model.FileReadRequest{Host: model.LocalHost, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if read.Content != "hello bridge\n" || read.Truncated {
		t.Fatalf("read = %#v", read)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o640 {
		t.Fatalf("mode = %o, want 640", mode)
	}
}

func TestReplaceRejectsAmbiguousText(t *testing.T) {
	root := t.TempDir()
	service, err := New(filepath.Join(root, "config.json"), filepath.Join(root, "ssh"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "repeated.txt")
	if err := os.WriteFile(path, []byte("same same"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = service.ReplaceFile(model.FileReplaceRequest{Host: model.LocalHost, Path: path, OldText: "same", NewText: "new"})
	if err == nil {
		t.Fatal("ambiguous replacement succeeded")
	}
}
