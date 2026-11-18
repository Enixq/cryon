//go:build windows

package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestUpdateHelperReplacesExecutableAndRestarts(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "обновление с пробелами")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "started.txt")
	t.Setenv("CRYON_UPDATE_TEST_MARKER", marker)
	source := filepath.Join(dir, "new.exe")
	program := filepath.Join(dir, "main.go")
	code := `package main
import "os"
func main() { _ = os.WriteFile(os.Getenv("CRYON_UPDATE_TEST_MARKER"), []byte("started"), 0600) }
`
	if err := os.WriteFile(program, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("go", "build", "-o", source, program).CombinedOutput(); err != nil {
		t.Fatalf("build test executable: %v: %s", err, output)
	}
	target := filepath.Join(dir, "Cryon test.exe")
	if err := os.WriteFile(target, []byte("old version"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := startUpdateHelper(source, target, 0); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(marker); err == nil && string(data) == "started" {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("updated executable did not start")
}
