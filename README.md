# glider

A compact multi-protocol forward proxy with flexible proxy chains, rule-based routing, health checking, DNS/DHCP features, and a built-in WebUI.

This repository is a maintained fork derived from [nadoo/glider](https://github.com/nadoo/glider) and [0xec/glider](https://github.com/0xec/glider). It keeps glider's lightweight design while adding practical observability for latency-based high availability (LHA) operation.

## v0.21.0 highlights

- Daily LHA availability statistics for groups and their forwarders.
- Per-forwarder daily availability and 15-minute availability/latency timelines, with jitter and the most/least stable windows shown in the summaries.
- New WebUI history page at `/lha` and query API at `/api/lha/history?date=YYYY-MM-DD`.
- Persistent daily JSONL history, separated automatically by Glider instance, with configurable retention (30 local dates by default).

## Forwarder aliases

Forwarder options use the existing `#OPTIONS` query syntax. Add `name=` to give a node a readable display name:

```ini
forward=trojan://user:pass@example.com:443#name=US-Trojan
forward=socks5://127.0.0.1:1080#priority=10&name=Local-SOCKS
forward=anytls://password@example.com:443#name="AnyTLS US"
```

Quoted names are accepted; surrounding quotes are removed for display.

Without `name=`, glider falls back to the original forwarder address, so existing configurations remain compatible.

## LHA mode

```ini
strategy=lha
check=http://clients3.google.com/generate_204#expect=204
checkinterval=10
checklatencysamples=6
checktolerance=50
```

The v0.20.x releases added runtime visibility around LHA. v0.21.0 adds daily availability history; it does not change the LHA scheduler.

After the first real connection, the log records the selected node:

```text
[lha] main: selected tuic US (181ms)
```

A later change is logged only when the selected forwarder changes:

```text
[lha] main: switch tuic US (260ms) -> hysteria UDP US (183ms)
```

Before any traffic has passed through an LHA group, the WebUI intentionally does **not** guess a current node. It displays `--` and `Waiting for first connection`.

## WebUI

Enable the web admin interface in the config:

```ini
web=:8888
```

Then open:

- `/status` — forwarder health, latency, strategy and current LHA selection
- `/lha` — daily LHA availability by group/node, with best/worst time windows and latency waveforms
- `/traffic` — traffic counters grouped by source IP
- `/logs` — recent in-memory logs
- `/api/status` — JSON status API
- `/api/lha/history?date=YYYY-MM-DD` — daily LHA probe history and 15-minute node buckets
- `/api/traffic` — JSON traffic API
- `/api/logs?limit=100` — JSON log API

The WebUI is embedded into the glider binary; no separate web files or web server are required at runtime.

### LHA history storage

History is appended to one `YYYY-MM-DD.jsonl` file per local date. By default, Glider writes it under a per-instance `glider-lha-history-<id>` directory in the configuration directory. The stable instance ID is derived from the listener, Web/DNS address and service settings.

- `GLIDER_LHA_HISTORY_PATH` overrides the history directory. Relative paths are resolved from Glider's working directory. Give each instance a different path when overriding this variable.
- `GLIDER_LHA_HISTORY_RETENTION_DAYS` sets how many local dates to keep. The default is `30`; set it to `0` to disable automatic cleanup.

Expired date files are removed at startup and when a new day's history file is opened. Cleanup only targets regular files named `YYYY-MM-DD.jsonl`.

Node availability is the successful-check percentage. Group availability is the percentage of observed 15-minute windows in which at least one node passed its check. Windows with no probe data remain gaps in the timeline. The most stable window ranks availability first, then lower latency variation and lower average latency.

## Scheduling strategies

| Strategy | Meaning |
| --- | --- |
| `rr` | Round robin |
| `ha` | High availability |
| `lha` | Latency-based high availability |
| `dh` | Destination hashing |

## Protocols

glider supports protocol conversion and chaining across a broad set of listeners and forwarders, including HTTP, SOCKS4/4A/5, Shadowsocks, Trojan, VLESS, VMess, SSH, TLS, WebSocket, KCP, Unix sockets, UDP/TCP tunnels, AnyTLS and others supported by the underlying project.

For detailed protocol URI syntax and configuration examples, see the files under [`config/examples`](config/examples) and use:

```bash
glider -scheme all
glider -example
glider -help
```

## Build from source

Go version requirements are defined by `go.mod`. A current stable Go toolchain is recommended.

```bash
go build -trimpath -ldflags="-s -w" -o glider .
```

On Windows, clone the repository and double-click:

```text
build-windows.bat
```

The executable is written to:

```text
build\glider.exe
```

The `build/` directory is ignored by Git.

## Basic usage

```bash
glider -verbose -listen :8443
glider -verbose -listen :8443 -forward socks5://127.0.0.1:1080
glider -verbose -config glider.conf client
```

## Compatibility

The `name=` option is display metadata only. Existing forwarder URLs without a name continue to work and use their address as the display label.

The v0.20.x and v0.21.0 LHA changes are observational: health checks and the existing LHA scheduling algorithm are preserved. History storage failures do not stop the proxy from running.

## Changelog

See [CHANGELOG.md](CHANGELOG.md).

## Credits

- Original project: [nadoo/glider](https://github.com/nadoo/glider)
- Fork lineage / WebUI work: [0xec/glider](https://github.com/0xec/glider)

## License

This project retains the license of the upstream glider project. See [LICENSE](LICENSE).
