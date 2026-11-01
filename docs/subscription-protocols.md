# 协议与订阅转换适配（0.22 后续测试版）

本测试版沿用 0.22 的订阅配置、更新周期、失败重试、缓存和 Web Status 页面。
不需要转换网站或额外代理内核进程。REALITY / Vision 在 Glider 进程内运行。

## 支持范围

| 分享链接 | 本次处理 | 范围 |
|---|---|---|
| SS / SSR | 保持现有转换 | 原有算法、协议与混淆限制不变 |
| AnyTLS | 新增订阅映射 | 现有 AnyTLS TLS 客户端 |
| Trojan | 新增订阅映射 | TCP + TLS；可校验证书 SHA-256 指纹 |
| VLESS | 新增订阅映射和客户端能力 | TCP 明文、TCP + TLS、TCP + REALITY；TLS / REALITY 支持 Vision |
| Hysteria2 / VMess 等 | 统计并跳过 | 未新增这些协议的订阅转换 |

未知参数、未知传输及不支持的安全模式不做降级转换。
已识别但不支持的配置会计数跳过；损坏 URL、非法端口、冲突的 SNI/证书校验别名
会使该份订阅整体失败，并保留上次可用结果。

Trojan / AnyTLS 暂不转换 WebSocket、gRPC、自定义 ALPN 等扩展。
VLESS 暂不转换 WS、gRPC、XHTTP、非 none encryption、其他 flow。

## 按协议测试 LHA

在实例主配置加入可选协议白名单（留空/不配置表示不限制）：

```ini
# 每次选一种协议测试；修改后重启该实例
subscription-protocol=anytls
# subscription-protocol=trojan
# subscription-protocol=vless

# 也支持多个协议，逗号分隔或重复配置，取并集
# subscription-protocol=anytls,trojan
```

支持 `ss`、`ssr`、`anytls`、`trojan`、`vless`，忽略大小写和两侧空格，
不认识的名称启动时会报错，避免拼写错误导致不受限。
协议筛选与 `subscription-match` / `subscription-exclude` 同时生效。
LHA 仍在筛选后的节点中自动择优；Web Status 的匹配节点数也使用同样的筛选。
VLESS 选择包含其受支持的各种传输；它不是只选 REALITY / Vision 的子模式筛选。

协议筛选仅作用于订阅节点，包括启动时恢复的旧快照；原始下载缓存保持共享，
不同实例可以选不同协议，无需重复下载。筛选后无有效候选时保留当前可用结果，
冷启动无匹配快照且无静态节点时保持 fail-closed，等待有效订阅。

**专门测协议的实例请移除手写 `forward=` / 旧节点 `include`，并避免用单独规则组
分流测试流量。** 这些静态节点及规则不受 `subscription-protocol` 影响。
确认 Web 页面里仅有目标协议节点后再测试。

## 参数映射

| 分享链接参数 | Glider 配置 |
|---|---|
| `#节点名称` | `#name=URL编码名称`，用于筛选和 Web Status |
| `sni` / `peer` / `servername` / `serverName` | `serverName`，非空别名冲突时拒绝 |
| `insecure` / `allowInsecure` / `skipVerify` | `skipVerify=true/false`；不主动放宽证书校验 |
| Trojan `pcs` | 保留并校验十六进制 SHA-256 证书指纹；即使 skipVerify=true 也必须匹配 |
| VLESS `encryption=none` | 基础 VLESS，不启用额外加密 |
| VLESS `security=none/tls/reality` | 对应的实际传输；绝不退回明文 |
| VLESS `flow=xtls-rprx-vision` | Vision；要求 TLS 1.3 或 REALITY |
| VLESS `pbk` / `sid` | REALITY X25519 公钥 / short ID |
| VLESS `fp` | chrome（默认）、firefox、safari、edge、ios |
| VLESS `alpn` | 逗号分隔的 ALPN 值 |
| VLESS TCP `mode` | 部分导出器多余的 XHTTP 字段；TCP 下不使用 |
| VLESS `spx` | 保留兼容字段；本实现认证失败即关闭，不执行爬虫回退 |

Trojan 指纹匹配叶证书时接受该指纹；匹配 CA 时还会校验证书链及 serverName。
当前每节点支持一个十六进制 `pcs` 指纹，不接纳多指纹列表或其他编码。

REALITY 必须有 serverName 和有效的 32 字节 Base64URL 公钥；short ID 为至多
8 字节的偶数位十六进制字符串，可为空。不接受跳过 REALITY 身份认证。
Vision TCP 与 XUDP 均已进行本地互通测试。

## 构建

正式构建脚本、Docker 与 GoReleaser 已添加 `with_utls`。
手动构建请用：

```sh
go build -tags with_utls -trimpath -ldflags="-s -w" .
TZ=UTC go test -race -tags with_utls ./...
go vet -tags with_utls ./...
```

不带此 tag 也能编译，但不启用 REALITY，相关订阅节点会明确跳过。
TLS / Vision 由 sing-vmess v0.2.7 实现；REALITY 握手基于 XTLS / sing-box 的
认证格式，uTLS 固定为 github.com/metacubex/utls v1.7.3。
Vision 客户端代码在 `proxy/vless/internal/vision` 保留带许可证的兼容副本，
修正上游 uintptr 指针往返以通过 Go checkptr / race，协议格式不变。
这些依赖均编译进入单个可执行文件，无须在 N1 安装 Go 或 Xray。

## 已验证与待确认

- 用户样本离线导入：28 SS + 12 AnyTLS、28 Trojan、33 VLESS 全部通过；
  另 6 个 Hysteria2 明确跳过。用户凭据未写入仓库或测试包。
- AnyTLS / Trojan：本地 TLS 连接及大段 HTTP 数据传输通过；Trojan 指纹匹配和拒绝通过。
- REALITY + Vision：使用独立 Xray 1.8.24 服务端，五种指纹的大段 HTTPS
  数据传输通过，错误 short ID 被拒绝；Vision XUDP 本地回显通过。
- 可选独立互通测试：设置 `GLIDER_XRAY_TEST_BINARY` 为 Xray 可执行文件路径；
  测试仅在回环地址启动临时服务，不会使用真实订阅或访问机场。
- N1 上实际机场连通性、吞吐、内存占用及长期连接稳定性仍待实机确认。

## N1 最小测试

1. 备份正在使用的 0.22 二进制、主配置及 subscription-output 指定的文件。
2. 用测试二进制替换运行文件（保留原配置和 include 顺序）。
3. 更新含 AnyTLS / Trojan / VLESS 的订阅，查看 `-verbose` 和 Web Status。
4. 确认节点名称、区域筛选、成功时间及节点可用状态；每种协议选一个实际访问测试。
5. 运行一段时间，检查长连接、重连和资源占用。

回退：停止测试实例，恢复原二进制及备份的 subscription-output，然后按原方式启动。
若原订阅已换成新协议，回退时也恢复原 SS/SSR 订阅配置，避免 0.22 无有效候选。
