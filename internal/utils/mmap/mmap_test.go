package mmap

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpenSharedMappingAndReplace(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("当前实现只在 Linux 启用 mmap")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "cache")
	if err := os.WriteFile(path, []byte("old-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 8)
	if _, err = first.ReadAt(buf, 0); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "old-data" {
		t.Fatalf("unexpected first mapping: %q", buf)
	}

	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	buf = make([]byte, 8)
	if _, err = second.ReadAt(buf, 0); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "old-data" {
		t.Fatalf("shared mapping was released too early: %q", buf)
	}

	replacement := path + ".new"
	if err = os.WriteFile(replacement, []byte("new-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}

	third, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	buf = make([]byte, 8)
	if _, err = third.ReadAt(buf, 0); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "new-data" {
		t.Fatalf("replacement file reused old mapping: %q", buf)
	}

	buf = make([]byte, 8)
	if _, err = second.ReadAt(buf, 0); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "old-data" {
		t.Fatalf("old reader was affected by replacement: %q", buf)
	}

	_ = second.Close()
	_ = third.Close()
}
