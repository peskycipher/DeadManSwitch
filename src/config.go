package main

import (
	"fmt"
	"github.com/BurntSushi/toml"
)

type config struct {
	TrySystemResolver bool     `toml:"try_system_resolver"`
	CustomResolvers   []string `toml:"custom_resolvers"`
	Record            string   `toml:"record"`
	RecordType        string   `toml:"record_type"`
	ExpectedValue     string   `toml:"expected_value"`
	DeleteFiles       []string `toml:"delete_files"`
	ExecuteScripts    []string `toml:"execute_scripts"`
	Countdown         uint     `toml:"countdown"`
	CheckInterval     uint     `toml:"check_interval"`
	DryRun            bool     `toml:"dry_run"`
	ExitAfterTrigger  bool     `toml:"exit_after_trigger"`
}

func loadConfig(path string) (*config, error) {
	conf := &config{}
	metaData, err := toml.DecodeFile(path, conf)
	if err != nil {
		return nil, err
	}
	for _, key := range metaData.Undecoded() {
		return nil, &configError{fmt.Sprintf("unknown option %q", key.String())}
	}

	if conf.CheckInterval == 0 {
		conf.CheckInterval = 60
	}
	if conf.Countdown == 0 {
		conf.Countdown = 3600
	}

	return conf, nil
}

type configError struct {
	err string
}

func (e *configError) Error() string {
	return e.err
}
