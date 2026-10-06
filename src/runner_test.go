package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Characterization: a single executable file runs; its effect is observable.
func TestRunScriptExecutesFile(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "marker")
	dir := t.TempDir()
	script := filepath.Join(dir, "hook.sh")
	//nolint:gosec // test fixture
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0755); err != nil {
		t.Fatal(err)
	}

	runScriptIterative(script)

	if _, err := os.Stat(marker); err != nil {
		t.Errorf("script did not run: %v", err)
	}
}

// Characterization: a directory is recursed; files in subdirectories execute too.
func TestRunScriptRecursesDirectory(t *testing.T) {
	base := t.TempDir()
	sub := filepath.Join(base, "hooks")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "marker")
	//nolint:gosec // test fixture
	if err := os.WriteFile(filepath.Join(sub, "hook.sh"), []byte("#!/bin/sh\ntouch "+marker+"\n"), 0755); err != nil {
		t.Fatal(err)
	}

	runScriptIterative(base)

	if _, err := os.Stat(marker); err != nil {
		t.Errorf("script in subdirectory did not run: %v", err)
	}
}

// Characterization: a missing path is skipped silently (no panic, no error return).
func TestRunScriptMissingPathSkips(t *testing.T) {
	runScriptIterative(filepath.Join(t.TempDir(), "does-not-exist")) // no panic = pass
}

// Characterization: a non-executable file fails exec internally; error is logged, not escalated.
func TestRunScriptNonExecutableFileLoggedNotFatal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-executable.sh")
	if err := os.WriteFile(path, []byte("echo hi\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runScriptIterative(path) // no panic = pass; exec error was swallowed by design
}

// Characterization: a file is removed.
func TestDelFileRemovesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	delFileIterative(path)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still present after delete: %v", err)
	}
}

// Characterization: a directory tree is removed bottom-up.
func TestDelFileRemovesTree(t *testing.T) {
	base := t.TempDir()
	deep := filepath.Join(base, "l1", "l2")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}
	leaf := filepath.Join(deep, "leaf")
	if err := os.WriteFile(leaf, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	delFileIterative(filepath.Join(base, "l1"))
	if _, err := os.Stat(filepath.Join(base, "l1")); !os.IsNotExist(err) {
		t.Errorf("tree still present after delete: %v", err)
	}
}

// Characterization: deleting a missing path is a silent no-op.
func TestDelFileMissingPathNoOp(t *testing.T) {
	delFileIterative(filepath.Join(t.TempDir(), "never-existed")) // no panic = pass
}

// Characterization of the trigger decision (extracted from check()):
// trigger beats expected; expected beats nothing; any match beats Uncertain.
func TestEvaluateRecords(t *testing.T) {
	conf := &config{ExpectedValue: "expected", TriggerValue: "trigger"}

	cases := []struct {
		name    string
		records []string
		want    Triool
	}{
		{"no match at all is Uncertain", []string{"unrelated", "also-unrelated"}, Uncertain},
		{"expected value is False", []string{"has-expected-inside"}, False},
		{"trigger value is True", []string{"has-trigger-inside"}, True},
		{"trigger on same record as expected wins", []string{"expected-and-trigger"}, True},
		{"trigger after expected wins", []string{"expected", "trigger-later"}, True},
		{"expected later still False, no trigger", []string{"first", "later-expected"}, False},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := evaluateRecords(tc.records, conf)
			if got != tc.want {
				t.Errorf("evaluateRecords(%q) = %v, want %v", tc.records, got, tc.want)
			}
		})
	}
}
