package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

// Characterization of the decision core (extracted from check()):
// expected value present → False (alive); record visible but expected absent → True (missing).
// Uncertain is a transport-level state (lookup failed) and is never produced here.
func TestEvaluateRecords(t *testing.T) {
	conf := &config{ExpectedValue: "expected"}

	cases := []struct {
		name    string
		records []string
		want    Triool
	}{
		{"expected present is False", []string{"has-expected-inside", "other"}, False},
		{"visible but expected missing is True", []string{"unrelated", "also-unrelated"}, True},
		{"empty result set is missing/True", nil, True},
		{"expected on a later record is False", []string{"first", "later-expected"}, False},
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

// Characterization of the countdown state machine (advance, wired into main):
// False cancels; True arms, fires when expired, re-arms after firing;
// Uncertain freezes an armed countdown by exactly the elapsed poll period.
func TestAdvanceCountdown(t *testing.T) {
	conf := &config{Countdown: 3600}
	now := time.Unix(1_000_000, 0)
	lastTick := now.Add(-60 * time.Second) // a poll period elapsed since previous check

	t.Run("True arms a fresh full countdown", func(t *testing.T) {
		dl, fire := advance(time.Time{}, True, conf, now, lastTick)
		if fire || !dl.Equal(now.Add(3600*time.Second)) {
			t.Errorf("advance: dl=%v fire=%v, want dl=now+3600 no fire", dl, fire)
		}
	})

	t.Run("True while armed, not expired: keep deadline", func(t *testing.T) {
		armed := now.Add(600 * time.Second)
		dl, fire := advance(armed, True, conf, now, lastTick)
		if fire || !dl.Equal(armed) {
			t.Errorf("advance: dl=%v fire=%v, want unchanged dl, no fire", dl, fire)
		}
	})

	t.Run("True past deadline fires and re-arms", func(t *testing.T) {
		armed := now.Add(-1 * time.Second)
		dl, fire := advance(armed, True, conf, now, lastTick)
		if !fire {
			t.Error("advance: no fire past deadline")
		}
		if !dl.Equal(now.Add(3600 * time.Second)) {
			t.Errorf("advance: dl=%v, want re-armed now+3600", dl)
		}
	})

	t.Run("False cancels the countdown", func(t *testing.T) {
		armed := now.Add(600 * time.Second)
		dl, fire := advance(armed, False, conf, now, lastTick)
		if fire || !dl.IsZero() {
			t.Errorf("advance: dl=%v fire=%v, want zero dl", dl, fire)
		}
	})

	t.Run("Uncertain freezes an armed countdown by the elapsed period", func(t *testing.T) {
		armed := now.Add(600 * time.Second)
		dl, fire := advance(armed, Uncertain, conf, now, lastTick)
		if fire {
			t.Error("advance: fire on uncertain")
		}
		if !dl.Equal(armed.Add(60 * time.Second)) {
			t.Errorf("advance: dl=%v, want frozen dl+60s", dl)
		}
	})

	t.Run("Uncertain with nothing armed does nothing", func(t *testing.T) {
		dl, fire := advance(time.Time{}, Uncertain, conf, now, lastTick)
		if fire || !dl.IsZero() {
			t.Errorf("advance: dl=%v fire=%v, want zero dl no fire", dl, fire)
		}
	})
}
