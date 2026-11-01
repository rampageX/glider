# glider

[English](README.en.md) | 简体中文

轻量级多协议转发代理，支持灵活的代理链、规则路由、健康检查、DNS/DHCP 和内置 Web 管理界面。

本仓库基于 [nadoo/glider](https://github.com/nadoo/glider) 与 [0xec/glider](https://github.com/0xec/glider) 维护，在保留 Glider 轻量设计的基础上，增加节点名称、LHA 运行状态与历史统计，以及原生订阅管理。

## v0.23.0 更新亮点

**五种订阅协议、REALITY + Vision，以及更方便的 LHA 测试和状态识别。**

- **订阅转换扩展：** 支持 SS、SSR、AnyTLS、Trojan TCP/TLS、VLESS TCP（明文 / TLS / REALITY）；TLS / REALITY 可使用 Vision。无需转换网站或外挂 Xray 进程。
- **协议筛选：** `subscription-protocol=anytls`、`trojan` 或 `vless` 可限定 LHA 候选；支持逗号分隔或重复配置多个协议，留空不限制。common 和实例配置均可使用，重复项取并集。
- **时间后缀：** 间隔、超时和 TTL 可写 `1m`、`2h`、`1d`、`1w`，裸数字仍按秒。**`checktolerance` 裸数字仍为毫秒**，`1000` 与 `1s` 等价。
- **状态页协议栏：** 从转发配置识别协议，代理链显示如 `TLS → WS → VLESS`，不依赖节点名称。
- **沿用 0.22 的订阅能力：** 多订阅、区域筛选、原子输出与运行中更新、共享缓存、优先级下载路径、订阅状态和默认两小时失败重试。

用户已确认 **AnyTLS、Trojan、VLESS REALITY + Vision 实机连接可用**。速度、资源占用与长期稳定性仍需持续观察；其他构建架构未逐一实机验证。

[下载 v0.23.0](https://github.com/rampageX/glider/releases/tag/v0.23.0) · [N1 / Linux ARM64 安装包](https://github.com/rampageX/glider/releases/download/v0.23.0/glider_0.23.0_linux_arm64.tar.gz) · [版本说明](docs/releases/v0.23.0.md)

## 原生订阅（SS / SSR / AnyTLS / Trojan / VLESS）

每个实例独立管理区域、节点输出文件及运行中的节点列表。多个实例可以通过公共配置共享订阅地址、下载设置和原始订阅缓存。

### 快速开始

请替换占位订阅地址，并按实际部署调整代理路径、目录和监听地址。以下示例保留名称/分组包含“香港”或“日本”的节点，排除“遊戲”：

```ini
listen=127.0.0.1:7575
web=127.0.0.1:8885
strategy=lha

subscription-title=主订阅
subscription=https://subscription.example.invalid/REPLACE_TOKEN
subscription-region=asia
# 可选：仅接纳指定协议；留空不限制
# subscription-protocol=vless

subscription-match=香港
subscription-match=日本
subscription-exclude=遊戲
subscription-output=/var/lib/glider/nodes-asia.conf

subscription-proxy=direct://#priority=100
subscription-proxy=socks5h://127.0.0.1:1080#priority=90

subscription-interval=1h
subscription-cache-ttl=1h
subscription-retry-interval=2h
subscription-cache-dir=/tmp/.subscription-cache
lha-history-dir=/tmp/.glider-history
```

使用 `glider -config /path/to/glider.conf` 启动，然后在同一台机器打开 `http://127.0.0.1:8885/status`。运行用户需要对节点输出、缓存与历史目录具有写入权限。

- 多个订阅重复填写 `subscription=`。标题可选；使用标题时，`subscription-title=` 必须与订阅数量一致，按顺序一一对应。
- 下载路径复用 Glider 的 forwarder 语法：`#priority=` 数值越大越先尝试，同优先级按声明顺序。`direct://` 明确表示直连；`socks5h://` 由 SOCKS 服务器解析目标域名。
- **正常刷新周期、缓存 TTL、失败重试间隔是三个独立设置。** 有效缓存可以直接复用，读取不会延长有效期；部分失败后重试时，成功输入的有效缓存仍可复用。
- 启动时若存在上次成功生成的节点文件，先恢复服务，再后台刷新；首次安装即使没有旧节点文件，也能获取订阅并开始转发。
- 筛选采用区分大小写的字面子串匹配：SS 匹配节点名，SSR 匹配 group + remarks。本版转换 SS/SSR、AnyTLS、Trojan 与受支持的 VLESS 配置；可识别但不支持的分享协议会统计并跳过。
- Linux 下可用 `systemctl kill -s HUP glider` 请求名为 `glider` 的服务手动刷新，仍遵守缓存 TTL。Status 页面的 **Refresh now** 按钮只刷新页面显示的数据。

### 新参数与时间单位

```ini
# 只接纳订阅中的 VLESS；也可写 anytls,trojan，或重复配置
subscription-protocol=vless
# subscription-protocol=anytls,trojan

checkinterval=1m
checktimeout=3
# 毫秒，与 1s 等价
checktolerance=1000
subscription-interval=2h
subscription-cache-ttl=1d
subscription-retry-interval=2h
```

时间单位：`s` 秒、`m` 分钟、`h` 小时、`d` 天（24 小时）、`w` 周（7 天），支持 `1h30m`。
普通时间配置裸数字为秒；`checktolerance` 裸数字为毫秒且可用 `ms`。
DHCP `LEASE_MINUTES` 裸数字仍为分钟，历史保留 `GLIDER_LHA_HISTORY_RETENTION_DAYS` 裸数字仍为天。
主配置、common include、规则文件和命令行都支持时间后缀；数值必须符合原字段的精度，不能表示为整数秒的普通配置会报错。

协议筛选与区域关键词同时生效，重启恢复快照时也会过滤。它**只限制订阅节点**，不影响手写 `forward=`、旧节点 include 或独立规则组；专门测协议时请移开这些额外节点和分流规则。修改配置后重启实例，普通订阅更新仍可自动应用。

### 从静态配置迁移与四实例部署

静态 `forward=`、普通 `include=` 和规则文件仍然受支持。对于订阅管理的节点文件，将原来的 `include=` 改为 `subscription-output=`，**同一文件不能同时使用这两种声明**。切换前禁用对应的旧脚本/cron 更新任务，并保留备份以便回退。

公共 `include=` 放在实例专有设置之前。include 在声明位置展开；列表按顺序追加，标量由后面的值覆盖，命令行参数最后应用。普通 include 文件缺失仍会报错。

详见[完整配置、迁移与回退指南](docs/native-subscriptions.md)，以及[公共配置和 asia/hk/jp/us 四实例示例](config/subscription)。

## 节点名称

在现有 forwarder 的 `#OPTIONS` 参数中加入 `name=`，即可设置便于辨认的显示名称：

```ini
forward=trojan://password@example.com:443#name=US-Trojan
forward=socks5://127.0.0.1:1080#priority=10&name=Local-SOCKS
forward=anytls://password@example.com:443#name="AnyTLS US"
```

名称支持外层引号，显示时会移除。未设置 `name=` 时，仍使用原 forwarder 地址作为显示名称，兼容已有配置。

## LHA 模式

```ini
strategy=lha
check=http://clients3.google.com/generate_204#expect=204
checkinterval=10
checklatencysamples=6
checktolerance=50
```

v0.20.x 增加 LHA 运行状态展示，v0.21.0 增加每日可用率历史；这些观测功能没有改变 LHA 调度算法。

首次真实连接后，日志记录实际选中的节点：

```text
[lha] main: selected tuic US (181ms)
```

后续仅在选中节点发生变化时记录切换：

```text
[lha] main: switch tuic US (260ms) -> hysteria UDP US (183ms)
```

某个 LHA 分组尚无真实流量时，Web 页面不会猜测当前节点，而是显示 `--` 和 `Waiting for first connection`。

## Web 管理界面

在配置中启用：

```ini
web=:8888
```

可访问以下页面与接口：

- `/status`：节点健康状态、延迟、当前 LHA 节点，以及带标题和刷新时间的订阅结果。
- `/lha`：分组/节点每日可用率、最佳/最差时间窗口及延迟波形。
- `/traffic`：按来源 IP 分组的流量统计。
- `/logs`：近期内存日志。
- `/api/status`：节点与订阅状态 JSON 接口。
- `/api/lha/history?date=YYYY-MM-DD`：每日 LHA 探测历史与 15 分钟节点统计。
- `/api/traffic`：流量 JSON 接口。
- `/api/logs?limit=100`：日志 JSON 接口。

Web 页面已嵌入二进制文件，运行时无需单独部署网页文件或 Web 服务器。

### LHA 历史存储

历史按本地日期追加到 `YYYY-MM-DD.jsonl`。从 v0.23.0 起，Linux 默认基础目录为 `/tmp/.glider-history`，每个实例使用独立的 `glider-lha-history-<id>` 子目录。稳定实例 ID 由监听地址、Web/DNS 地址和服务设置生成。Windows 默认使用系统临时目录作为基础目录。

若需要在临时目录清理或重启后保留历史，可指定持久化基础目录：

```ini
lha-history-dir=/var/lib/glider/history
```

相对路径以主配置文件所在目录为基准。已有历史不会自动搬迁。

- 非空的 `GLIDER_LHA_HISTORY_PATH` 环境变量优先于 `lha-history-dir`，指定完整的实例历史目录；其相对路径以 Glider 工作目录为基准。多个实例使用该变量时，应分别指定不同目录。
- `GLIDER_LHA_HISTORY_RETENTION_DAYS` 设置保留的本地日期数量，默认 `30`；设为 `0` 关闭自动清理。

启动及打开新一天的历史文件时，会清理过期日期文件。仅清理文件名为 `YYYY-MM-DD.jsonl` 的普通文件。

节点可用率是健康检查成功次数的百分比。分组可用率是已观测的 15 分钟窗口中，至少一个节点检查成功的窗口占比。没有探测数据的时段保留为空白。最稳定窗口优先按可用率排名，再比较延迟波动和平均延迟。

## 调度策略

| 策略 | 含义 |
| --- | --- |
| `rr` | 轮询 |
| `ha` | 高可用 |
| `lha` | 基于延迟的高可用 |
| `dh` | 目标地址哈希 |

## 协议支持

Glider 支持多种监听、转发、协议转换和代理链，包括 HTTP、SOCKS4/4A/5、Shadowsocks、Trojan、VLESS、VMess、SSH、TLS、WebSocket、KCP、Unix socket、UDP/TCP 隧道、AnyTLS 等底层项目支持的协议。

**转发协议支持与订阅转换支持不同：本版订阅转换支持 SS / SSR / AnyTLS / Trojan / VLESS 的指定配置。VMess、Hysteria2 等分享链接仍统计并跳过；WS / gRPC / XHTTP 等扩展限制见[协议与参数映射说明](docs/subscription-protocols.md)。**

详细协议 URI 语法与配置示例见 [config/examples](config/examples)，也可运行：

```bash
glider -scheme all
glider -example
glider -help
```

## 从源码构建

Go 版本要求见 `go.mod`，建议使用当前稳定版工具链。**REALITY 需要 `with_utls` 构建标签**；仓库构建脚本和正式发行包已启用，不带标签的自编译版本不支持 REALITY。

```bash
go build -tags with_utls -trimpath -ldflags="-s -w" -o glider .
```

Windows 下克隆仓库后，可以双击：

```text
build-windows.bat
```

生成文件位于：

```text
build\glider.exe
```

`build/` 已加入 Git 忽略列表。

## 基本用法

```bash
glider -verbose -listen :8443
glider -verbose -listen :8443 -forward socks5://127.0.0.1:1080
glider -verbose -config glider.conf client
```

## 兼容性

`name=` 仅为显示元数据。没有名称的旧 forwarder URL 继续有效，并使用地址作为显示名称。

v0.20.x 与 v0.21.0 的 LHA 改动增加观测能力；v0.23.0 增加订阅节点的运行时更新，保留现有调度策略。普通静态单节点分组仍跳过健康检查；订阅管理的主分组即使只有一个节点也进行健康检查，以支持后续 1↔N 节点更新。历史存储失败不会阻止代理运行。

## 更新记录

见 [CHANGELOG.md](CHANGELOG.md)。

## 致谢

- 原始项目：[nadoo/glider](https://github.com/nadoo/glider)
- Fork 来源与 WebUI 工作：[0xec/glider](https://github.com/0xec/glider)

## 许可证

沿用上游项目许可证，见 [LICENSE](LICENSE)。
