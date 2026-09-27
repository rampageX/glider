# Changelog

All notable changes in this fork are documented here.

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
