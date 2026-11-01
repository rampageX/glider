# glider

English | [简体中文](README.md)

A compact multi-protocol forward proxy with flexible proxy chains, rule-based routing, health checking, DNS/DHCP features, and a built-in WebUI.

This repository is a maintained fork derived from [nadoo/glider](https://github.com/nadoo/glider) and [0xec/glider](https://github.com/0xec/glider). It keeps glider's lightweight design while adding practical observability for latency-based high availability (LHA) operation.

## v0.22.0 highlights

**Native subscriptions, live node updates, and subscription status in the WebUI.**

The development test build adds **AnyTLS, Trojan and VLESS (TLS / REALITY + Vision) subscription conversion**. See [scope, build flags and validation](docs/subscription-protocols.md).

- **Built-in subscription management:** download and parse multiple SS/SSR subscriptions, preserve node names, and filter regions using configurable literal keywords. No external update script is required.
- **Update nodes while running:** generate standard `forward=` files and apply changes without restarting Glider. Identical definitions keep their health state; all-input failures or empty candidates retain the current nodes. Partial failures exclude the failed input.
- **Independent download fallback per subscription:** ordered direct/proxy GET attempts, a shared cross-process cache, and atomic cache/output writes.
- **Visible subscription health:** optional titles, masked URLs, individual success/failure results, check/download timestamps, and the next refresh time on Status.
- **Conservative failure retries:** a separate retry interval defaults to **7200 seconds (two hours)**; a shorter normal refresh interval does not bring a failed retry forward.
- **Configurable storage and diagnostics:** Linux cache defaults to `/tmp/.subscription-cache`, LHA history to `/tmp/.glider-history`; `-verbose` adds diagnostics without printing subscription URLs, tokens, or response bodies.

[Download v0.22.0](https://github.com/rampageX/glider/releases/tag/v0.22.0) · [N1 / Linux ARM64 package](https://github.com/rampageX/glider/releases/download/v0.22.0/glider_0.22.0_linux_arm64.tar.gz) · [Release notes](docs/releases/v0.22.0.md)

The test build also supports duration suffixes (for example `subscription-interval=2h`) and shows node protocols on Status. Bare `checktolerance` values remain milliseconds.

## Native SS/SSR subscriptions

Each instance can manage its own region and output file. Multiple instances can share a common subscription configuration and raw cache while keeping their runtime node lists independent.

### Quick start

Replace the placeholder URL and adjust the proxy route, paths and listeners for your deployment. This example selects names/groups containing `香港` or `日本`, excluding `遊戲`:

```ini
listen=127.0.0.1:7575
web=127.0.0.1:8885
strategy=lha

subscription-title=Primary subscription
subscription=https://subscription.example.invalid/REPLACE_TOKEN
subscription-region=asia
# Optional subscription protocol allowlist; empty permits all
# subscription-protocol=vless

subscription-match=香港
subscription-match=日本
subscription-exclude=遊戲
subscription-output=/var/lib/glider/nodes-asia.conf

subscription-proxy=direct://#priority=100
subscription-proxy=socks5h://127.0.0.1:1080#priority=90

subscription-interval=3600
subscription-cache-ttl=3600
subscription-retry-interval=7200
subscription-cache-dir=/tmp/.subscription-cache
lha-history-dir=/tmp/.glider-history
```

Run with `glider -config /path/to/glider.conf`, then open `http://127.0.0.1:8885/status` on the same machine. The process needs write access to the output, cache and history directories.

- Repeat `subscription=` for multiple inputs. Titles are optional; when used, provide one `subscription-title=` per input in the same order.
- Download routes use Glider's existing forwarder syntax: higher `#priority=` first, declaration order for ties. `direct://` explicitly means direct access; `socks5h://` sends destination names to the SOCKS server.
- The normal refresh interval, cache TTL and failure retry interval are **separate settings**. A valid cache can be reused without another GET; reading it does not extend its age. Successful cached inputs remain reusable during a partial-failure retry.
- Startup restores the last successful output file when available, then refreshes in the background. A first-time installation can fetch subscriptions and start forwarding without an old node file.
- Matching uses SS names or SSR group + remarks, with case-sensitive literal substrings. This development build converts SS/SSR, AnyTLS, Trojan and supported VLESS configurations; recognized unsupported sharing schemes are counted and skipped.
- On Linux, `systemctl kill -s HUP glider` manually requests a refresh for a service named `glider`; it still respects cache TTL. The Status page's **Refresh now** button only reloads the displayed status.

### Existing static configurations and multi-instance migration

Static `forward=` entries, ordinary includes and rule files remain supported. For a subscription-managed node file, replace its old `include=` entry with `subscription-output=`; **do not use both for the same file**. Disable the corresponding old script/cron updater before switching, and retain backups for rollback.

Place a shared `include=` before instance-specific settings. Includes expand at their declaration point; lists append in order, later scalars override earlier values, and CLI arguments are applied last. Missing ordinary include files remain errors.

See the [complete configuration, migration and rollback guide](docs/native-subscriptions.md) and the [shared + asia/hk/jp/us examples](config/subscription).

## Forwarder aliases

Forwarder options use the existing `#OPTIONS` query syntax. Add `name=` to give a node a readable display name:

```ini
forward=trojan://password@example.com:443#name=US-Trojan
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

- `/status` — forwarder health, latency, current LHA selection, and named subscription results with refresh timestamps
- `/lha` — daily LHA availability by group/node, with best/worst time windows and latency waveforms
- `/traffic` — traffic counters grouped by source IP
- `/logs` — recent in-memory logs
- `/api/status` — JSON forwarder and subscription status API
- `/api/lha/history?date=YYYY-MM-DD` — daily LHA probe history and 15-minute node buckets
- `/api/traffic` — JSON traffic API
- `/api/logs?limit=100` — JSON log API

The WebUI is embedded into the glider binary; no separate web files or web server are required at runtime.

### LHA history storage

History is appended to one `YYYY-MM-DD.jsonl` file per local date. Since v0.22.0, the default base directory is `/tmp/.glider-history`, with a separate `glider-lha-history-<id>` subdirectory for each instance. The stable instance ID is derived from the listener, Web/DNS address and service settings. On Windows, the default base uses the system temporary directory.

Set a persistent base directory when history must survive cleanup or reboot:

```ini
lha-history-dir=/var/lib/glider/history
```

Relative `lha-history-dir` paths resolve from the main configuration directory. Existing history is not moved automatically.

- `GLIDER_LHA_HISTORY_PATH`, when nonempty, takes precedence over `lha-history-dir` and specifies the full instance history directory. Relative paths are resolved from Glider's working directory. Give each instance a different path when overriding this variable.
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

The v0.20.x and v0.21.0 LHA changes added observability. v0.22.0 adds subscription-managed runtime membership changes while retaining the existing scheduling strategies. Ordinary static single-node groups still skip health checks; subscription-managed main groups check even a single node so later 1↔N updates remain supported. History storage failures do not stop the proxy from running.

## Changelog

See [CHANGELOG.md](CHANGELOG.md).

## Credits

- Original project: [nadoo/glider](https://github.com/nadoo/glider)
- Fork lineage / WebUI work: [0xec/glider](https://github.com/0xec/glider)

## License

This project retains the license of the upstream glider project. See [LICENSE](LICENSE).
