# Changelog

All notable changes in this fork are documented here.

## [Unreleased]

- Accept s/m/h/d/w time suffixes for interval, timeout and TTL settings, including common/rule configs, while preserving bare-number milliseconds for checktolerance and explicit legacy minute/day fields.
- Show each forwarder's protocol (and transport chains) in the Status page and API.

- Add optional `subscription-protocol` allowlist for LHA protocol testing; apply it to refreshes, matched-node status counts and restored snapshots without changing shared raw caches.

- Import AnyTLS, TCP/TLS Trojan and supported TCP VLESS subscription URIs, preserving names and normalizing SNI/verification aliases without transport downgrade.
- Add in-process VLESS TLS / REALITY + Vision clients, browser fingerprints and Vision XUDP; official build paths enable `with_utls`.
- Enforce Trojan SHA-256 certificate pins, including CA-chain validation, and close failed TLS handshakes.
- Keep a licensed, narrowly patched sing-vmess Vision client copy to preserve GC pointer tracking and pass checkptr/race instrumentation.
- Add independent Xray REALITY/Vision interoperability tests, imported TLS protocol transfer tests and protocol/rollback documentation.

## [0.22.0] - 2026-11-01

### Added

- Native SS/SSR subscriptions: multiple inputs, literal region filters, ordered direct/proxy fallback GET requests, and credential-safe verbose diagnostics.
- Shared, locked, atomic raw subscription caching and managed regional forward files that also serve as last-successful node snapshots.
- Transactional runtime main-group updates, preserving unchanged forwarder objects and existing connections; removed health-check tasks can exit.
- Mixed-subscription metadata/comment handling; known unsupported sharing schemes are counted and skipped without conversion.
- Optional subscription titles and a Status panel showing masked URLs, independent refresh outcomes, cache/download source, check/download timestamps and next refresh time.
- Configurable failure retries, defaulting to 7200 seconds (two hours), without earlier interruption by the normal refresh interval.
- Configurable `lha-history-dir`; cache and history defaults now use temporary directories instead of the main configuration directory.
- Complete shared/four-instance example configurations and migration/rollback instructions.

### Changed

- Main configuration includes expand in place and parse once: ordered list append, last scalar wins, CLI last, relative include paths based on the including file. Ordinary missing includes remain errors.
- SS unsupported ciphers return errors instead of terminating the process. Added the `socks5h` alias for the existing remote-DNS SOCKS5 dialer.
- Subscription-generated files use `subscription-output`, never a duplicate static include. Identical definitions do not rewrite files or reset health state.
- Releases include subscription documentation/examples and inject the package version into the executable.

### Validation

- Automated race, parser, proxy, subprocess cache-lock, cancellation, runtime replacement, TCP continuity, status and retry tests use local fixtures only.
- N1 ARM64 user testing confirmed startup without an old node file and the latest subscription/status build. Individual failing subscription services are not claimed universally reachable.
- Other release architectures are cross-compiled; they have not all been tested on physical hardware.

## [0.21.0] - 2026-09-27

### Added

- Daily LHA availability history for each group and forwarder.
- A WebUI history page at `/lha`, including per-node availability and 15-minute availability/latency timelines with jitter shown in the time-window summaries.
- Most and least stable time-window summaries for each forwarder.
- A date-query API at `/api/lha/history?date=YYYY-MM-DD`.
- Append-only daily JSONL history stored in a separate default directory for each Glider instance.
- `GLIDER_LHA_HISTORY_PATH` to override the history directory.
- `GLIDER_LHA_HISTORY_RETENTION_DAYS` to configure history retention; the default is 30 local dates and `0` disables cleanup.

### Changed

- Expired history files are cleaned up at startup and when a new local day's history file is opened. Cleanup only removes regular files whose names match `YYYY-MM-DD.jsonl`.

### Compatibility

- Existing LHA health checks and scheduling behavior are unchanged.
- History persistence failures do not prevent the proxy from starting.

## [0.20.1] - 2026-09-25

### Changed

- Split GoReleaser targets into dedicated Windows and Linux builds.
- Restored Linux multi-architecture releases for 386, amd64, ARMv5, ARMv6, ARMv7, ARM64 and RISC-V 64.
- ARMv5 is built with Go's software-float ABI; ARMv6/ARMv7 use the standard hard-float ABI.
- Kept Windows releases focused on amd64 (v1 and v4).

## [0.20.0] - 2026-09-17

### Added

- Forwarder display aliases through `#name=...`.
- Alias support alongside existing options, for example `#priority=10&name=US-Trojan`.
- Runtime tracking of the forwarder actually selected by LHA traffic.
- `[lha]` log entries for the initial selection and subsequent node changes.
- `name`, `current`, `current_latency_ms`, and runtime strategy information in the status API.
- Current-LHA presentation in the WebUI, including a `CURRENT` marker on the selected forwarder.
- Explicit idle LHA state before the first real connection: `-- / Waiting for first connection`.
- LHA group count in the WebUI summary.
- One-click Windows build helper with isolated `build/` output.

### Changed

- Updated the AnyTLS implementation with session reuse, legacy URL option compatibility, non-blocking stream setup, and per-stream deadline isolation.
- Human-facing health-check, group-status and forwarder failure logs prefer the configured display name while retaining address fallback compatibility.
- Quoted display names such as `name="AnyTLS TCP US"` are normalized by removing surrounding quotes.
- WebUI status terminology distinguishes an enabled LHA group from a group that has already handled traffic.
- The status API reports the scheduler strategy actually selected during group initialization rather than relying on shared configuration state.

### Compatibility

- The existing LHA scheduling algorithm and tolerance behavior are unchanged.
- `name=` is optional; configurations without it continue to display the original forwarder address.
- Existing `#priority=` and `#interface=` forwarder options remain compatible and may be combined with `name=` using `&`.

### Notes

`Current LHA` represents the last forwarder actually selected for a real connection. It is intentionally empty until the first connection passes through that LHA group; health-check probes alone do not create a current selection.
