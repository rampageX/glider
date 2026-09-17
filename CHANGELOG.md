# Changelog

All notable changes in this fork are documented here.

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

- - Updated the AnyTLS implementation with session reuse, legacy URL option compatibility, non-blocking stream setup, and per-stream deadline isolation.
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

 
