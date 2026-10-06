# Dead Man's Switch

This little daemon watches a DNS record periodically. While you are alive, the record contains a value known only to you (`expected_value`). When the record exists but no longer contains that value, a countdown starts; when it expires, the daemon runs a set of actions.

[![Build status](https://dev.azure.com/nekomimiswitch/General/_apis/build/status/DeadManSwitch)](https://dev.azure.com/nekomimiswitch/General/_build/latest?definitionId=1)

Currently supported triggers:
* Execute programs or scripts
* Delete files

## Installation

There are precompiled binaries for Linux in the [releases](https://github.com/Jamesits/DeadManSwitch/releases) page.

Requires:
* Go 1.22 or later (only needed for compilation)
* Cross-compiles with `make release` in `src/` for Linux, Windows, macOS, and FreeBSD; the runtime code uses only portable Go APIs. Only packaging/service installation is OS-specific (shell scripts + systemd on Linux).

There is currently no formal package. A `install.sh` can be used to compile from source and install to your localhost, and a `package.sh` can be used to generate a binary tarball.

## Config

By default (I mean, if you use the systemd service provided) the config is at `/etc/dmswitch/config.toml`, and the scripts or programs placed in `/etc/dmswitch/hooks` will be run once triggered. 

The config file is self-explanatory:

| Key | Meaning |
|---|---|
| `record` | DNS name to watch |
| `record_type` | `TXT`, `A` or `AAAA` — all are resolved with TXT lookups (the old reverse-lookup path was removed) |
| `expected_value` | substring that marks the record as "alive" |
| `try_system_resolver`, `custom_resolvers` | resolvers to try in order; first decisive answer wins |
| `countdown` | seconds between "value missing" and the switch firing; default 3600 |
| `check_interval` | seconds between polls; default 60 |
| `dry_run` | simulate firings: log what would run/be deleted, touch nothing |
| `delete_files`, `execute_scripts` | what runs/gets removed on firing |
| `exit_after_trigger` | exit after the first firing instead of continuing |

## How the countdown works

- Record contains `expected_value`: alive — any running countdown is cancelled and the persisted deadline is removed.
- Record resolvable but `expected_value` absent: the countdown is armed (or keeps running) and the deadline is persisted to `/tmp/dmswitch.countdown`, so a crash, restart or reboot does not reset it. On expiry the switch fires and re-arms as long as the value stays missing — a persistent miss fires roughly once per countdown period.
- Lookups fail (DNS unreachable): never arms the countdown, and never defers an armed one — the deadline keeps running on the wall clock, so an outage cannot hide an expired countdown. Only seeing `expected_value` again cancels it.

Caveats:

* Choose `expected_value` so it is unique to your record; any returned record string containing it counts as alive.
* All relative paths in the config are relative to the config file itself.
* Programs will be executed in alphabet order.
* File deletion happens after program execution.
* If you keep `exit_after_trigger = false`, the switch re-fires once per countdown period while the value stays missing — make your hooks idempotent.
* Set `dry_run = true` to rehearse: the full countdown state machine runs (including persistence), but firing only logs what would happen. Dry runs never exit, even with `exit_after_trigger = true`.

## Usage

There is a systemd service installed by default. You can use `systemctl enable --now dmswitch` to start it on boot.

If you prefer launching the binary directly, use `-conf path/to/config/file` to point it to the config file. If this parameter is missing, it searches for a `config.toml` in its working directory.

## Design

DNS is a perfect one-way channel for C&C. It doesn't require the program to connect to a specific server (recursion works most of the time), there are many free providers available, and there are APIs everywhere.
