package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConf(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Characterization: a valid file decodes into the struct fields.
func TestLoadConfigDecodesKnownOptions(t *testing.T) {
	path := writeConf(t, `
try_system_resolver = true
custom_resolvers = ["1.1.1.1", "8.8.8.8"]
record = "alive.example.com"
record_type = "TXT"
expected_value = "expected"
trigger_value = "trigger"
delete_files = ["/tmp/a"]
execute_scripts = ["/tmp/hook.sh"]
trigger_on_uncertain = true
max_uncertain_tolerance = 3
check_interval = 90
exit_after_trigger = true
`)
	conf, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !conf.TrySystemResolver || !conf.TriggerOnUncertain || !conf.ExitAfterTrigger {
		t.Errorf("bool fields wrong: %+v", conf)
	}
	if len(conf.CustomResolvers) != 2 || len(conf.DeleteFiles) != 1 || len(conf.ExecuteScripts) != 1 {
		t.Errorf("list fields wrong: %+v", conf)
	}
	if conf.Record != "alive.example.com" || conf.RecordType != "TXT" {
		t.Errorf("record fields wrong: %+v", conf)
	}
	if conf.ExpectedValue != "expected" || conf.TriggerValue != "trigger" {
		t.Errorf("value fields wrong: %+v", conf)
	}
	if conf.MaxUncertainTolerance != 3 || conf.CheckInterval != 90 {
		t.Errorf("numeric fields wrong: %+v", conf)
	}
}

// Characterization: check_interval = 0 defaults to 60.
func TestLoadConfigDefaultsCheckInterval(t *testing.T) {
	conf, err := loadConfig(writeConf(t, `record = "x"`))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if conf.CheckInterval != 60 {
		t.Errorf("CheckInterval = %d, want default 60", conf.CheckInterval)
	}
}

// Characterization: unknown keys are rejected, not ignored.
func TestLoadConfigRejectsUnknownOption(t *testing.T) {
	_, err := loadConfig(writeConf(t, `record = "x"
bogus_option = 1
`))
	if err == nil {
		t.Fatal("expected error for unknown option, got nil")
	}
	if got := err.Error(); got != `unknown option "bogus_option"` {
		t.Errorf("error = %q", got)
	}
}

// Characterization: record_type is stored as written; case handling lives in check().
func TestLoadConfigKeepsRecordTypeCase(t *testing.T) {
	conf, err := loadConfig(writeConf(t, `record_type = "txt"`))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if conf.RecordType != "txt" {
		t.Errorf("RecordType = %q, want as-written %q", conf.RecordType, "txt")
	}
}
