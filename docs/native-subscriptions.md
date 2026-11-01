# 原生订阅

> v0.23.0 支持 SS、SSR、AnyTLS、Trojan 与 VLESS 的指定配置。
> 支持用 `subscription-protocol=anytls`（或 trojan / vless）限定订阅协议。
> 新增参数映射、REALITY / Vision 范围及构建要求见[协议适配说明](subscription-protocols.md)。

此功能只更新默认 main 节点组。四个进程仍独立监听、筛选、生成文件和应用节点；
共享的是原始订阅缓存，不共享运行中节点。没有通用配置热重载，不调用外部下载工具、脚本或 systemctl。

## 配置文件

完整示例位于 `config/subscription/`：

| 文件 | 安装位置 | 本机代理 | Web | 区域匹配 | 输出 |
| --- | --- | --- | --- | --- | --- |
| common.conf.example | /root/cfg/subscription-common.conf | — | — | 默认排除 遊戲 | — |
| asia.conf.example | /root/cfg/glider.conf | :7575 | :8885 | 香港、台灣、日本、新加坡、韩国 | glider_db_asia_ssr.conf |
| hk.conf.example | /root/cfg/glider_hk.conf | :7776 | :8886 | 香港 | glider_db_hk_ssr.conf |
| jp.conf.example | /root/cfg/glider_jp.conf | :7777 | :8887 | 日本 | glider_db_jp_ssr.conf |
| us.conf.example | /root/cfg/glider_us.conf | :7778 | :8888 | 美国、美國 | glider_db_us_ssr.conf |

Web 端口是示例，部署时保留已有端口。也保留已有 listen、规则、DNS 等设置。
示例沿用 `:PORT` 监听方式；管理页面的访问范围请沿用现有部署策略。
订阅地址占位符必须在本地替换，按实际订阅数量删除或增加 `subscription=` 行。
配置、缓存、快照都含凭据，不要上传、贴出或加入 Git。

### 字段及默认值

| 字段 | 默认 | 含义 |
| --- | --- | --- |
| subscription-title | 可不写 | 可重复；按 subscription 顺序对应，设置时数量必须与订阅相同；空标题自动用编号 |
| subscription-retry-interval | 7200 | 部分/全部订阅失败或应用失败后的重试秒数；0 关闭额外重试 |
| subscription | 无 | 可重复；按声明顺序合并，各订阅独立尝试下载路径 |
| subscription-protocol | 空 | 可重复或逗号分隔；ss/ssr/anytls/trojan/vless 白名单，空表示不限制；common 与实例取并集；只筛选订阅节点和恢复快照 |
| subscription-region | 空 | 区域说明标签，不推断地理位置；筛选取决于 match/exclude |
| subscription-match | 空 | 可重复；大小写敏感字面子串 OR 匹配，空列表接受所有节点 |
| subscription-exclude | 遊戲 | 可重复；任意非空关键词命中就排除；显式设置替代默认，可用空值关闭排除 |
| subscription-output | 必填 | 本实例唯一的生成文件，同时是最后成功应用节点快照 |
| subscription-proxy | direct:// | 可重复；显式配置后替代默认列表，不隐式添加直连 |
| subscription-cache-dir | /tmp/.subscription-cache (Windows: TEMP 下同名目录) | 四实例应设置为同一绝对目录 |
| subscription-cache-ttl | 3600 | 原始订阅新鲜度秒数；0 表示每次刷新都下载 |
| subscription-interval | 3600 | 后台刷新周期秒数，与 TTL 独立，必须大于 0 |
| subscription-connect-timeout | 3 | 每条路径底层 TCP 连接超时秒数，必须大于 0 |
| subscription-timeout | 15 | 每条路径整次 GET 总超时秒数，包含代理握手、TLS、响应读取 |
| subscription-max-bytes | 8388608 | 解压后响应大小上限；超过整份拒绝 |

时间参数可使用 s/m/h/d/w（如 2h、1d），裸数字仍按秒；`checktolerance` 裸数字仍为毫秒。

SSR 用 `group + 空格 + remarks` 匹配，其余协议用片段名称匹配。关键词不拆分逗号、
不使用正则、不自动翻译、不按 IP 定位。多关键词请重复写字段。

### include 与覆盖规则（本版修正）

主配置使用现有 conflag 字段类型，先递归展开 include，再只解析一次：

1. `include=...` 在声明位置展开，嵌套相对 include 相对于包含它的文件目录。
2. include 可以使用绝对路径；普通 include 缺失、循环、过深都会报错。
3. 标量最后一次声明生效。因此示例把公共 include 放在主配置最前，本实例标量可以覆盖公共默认值。
4. `forward`、`subscription`、match/exclude/proxy 列表按声明顺序追加，保留重复项。
   列表的第一次显式赋值替代注册默认值。不存在清空继承列表的特殊语法。
5. listen、rulefile 等原来使用 uniq 类型的列表仍去重。
6. 命令行参数最后处理一次：标量覆盖文件，列表追加，不再因多次解析而重复追加。
7. `subscription-output`、cache-dir、rulefile、rules-dir 的相对路径均相对于主配置目录；
   include 内声明这些字段也一样，不相对于公共 include 目录。
8. 保留 `{$ENV_VAR}` 语法；值只展开一次，不能用环境变量递归生成新占位符。
9. rules-dir 及 `.rule` 内容的解析未改动，本版修正仅作用于主配置和它的 include。

这是对旧配置库“先解析主文件、再解析各 include、多次重放 CLI”的必要修正。
原先依赖 include 覆盖主文件的部署，应把对应 include 放在需要覆盖的值之后；
一般推荐示例中的“公共 include 在前，本机设置在后”。
启动读取完整配置后才准备订阅管理；本版不自动重读公共文件，改筛选配置后重启实例即可重新处理仍有效的缓存，无需下载。

### 下载路径

使用现有 Glider forwarder URL/链式语法及 `#priority=N`：**数值越大越先尝试**，
同优先级保持声明顺序。成功的完整 GET、解码和解析后停止尝试后续路径。
示例明确表达 直连 → PC → asia → hk → jp → us。
`socks5h://` 是现有 socks5 实现的别名，两者都发送域名给 SOCKS 服务端解析。
`direct://` 不读取 HTTP_PROXY/HTTPS_PROXY/NO_PROXY；显式代理同样不受环境变量改变。
不通过 HEAD 或第三方探测站判断订阅是否可用。

下载使用独立有界拨号链，不把运行中 main 组作为下载调度器。
选本机 SOCKS 路径时由现有监听器处理，它只转发 GET，不触发订阅刷新、不获取缓存锁，
因此不会形成“刷新等待自己的刷新锁”。尚未启动的代理连接失败后继续下一条；
已启动但尚无节点的实例使用拒绝节点，不能绕过代理偷偷直连。
每条路径都有总超时，最多尝试配置数量的路径；取消刷新会关闭下载握手套接字。
不要人为把订阅节点配置为回到自己的代理监听端口。

## 生成文件与更新语义

`subscription-output` 是专用入口，**不要再 include 同一路径**，否则启动明确报错。
普通静态 `forward` 与其他 include 不受影响，静态 forward 保留在 main 组前面；
空筛选结果不会移除当前订阅节点，也不会改变静态节点。

生成内容示例（仅虚构凭据，真实文件 0600）：

```ini
# Glider managed subscription snapshot; credentials; chmod 600; do not edit/include
forward=ss://aes-128-gcm:example-password@ss.example.invalid:443#name=%E9%A6%99%E6%B8%AF+Example
forward=ssr://aes-128-cfb:example-password@ssr.example.invalid:443?obfs=http_simple&obfs_param=example.invalid&protocol=auth_sha1_v4&protocol_param=123%3Aexample#name=%E6%97%A5%E6%9C%AC+Example
```

分享链接被规范化为 Glider 原生 URL，凭据和参数只解码一次再编码；名称使用 `#name=`。
编码后的逗号不会被当作转发链分隔符。名称的首尾空格和引号也保留；
URL 编码的 name 按 query 原样解码；未编码的手写 name 仍兼容旧版首尾空白/包裹引号处理。
普通静态 forward 仍使用原格式。
SS 插件、未知扩展、当前不支持的算法/SSR 协议或混淆明确计数并跳过。
其他格式损坏的节点导致整个订阅拒绝，不接受其余半份节点。
订阅转换支持 SS/SSR/AnyTLS/Trojan/VLESS，具体传输限制见顶部协议适配说明。不导出 JSON。

启动先校验已有输出文件，再作为上次成功快照恢复，随后监听服务、后台刷新。
生成文件首次不存在不报 include 错误；没有快照也没有静态节点时先拒绝转发，后台重试。
恢复明确记录 `freshness unknown`，不宣称缓存有效或刷新成功。
恢复文件可来自原脚本的纯 forward 文件，不能包含其他配置指令。

每个订阅按完整 URL 的 SHA-256 标识缓存，文件名和日志不出现 URL/token。
同机进程使用内核文件锁，获得锁后重新检查 TTL；进程崩溃锁自动释放。
缓存读取不修改时间；完整下载、解码和解析后才原子发布，不用过期缓存作为成功输入。
正常创建目录 0700、缓存/锁/输出文件 0600；已有目录权限由管理员保持适当设置。
共享目录应为 N1 本地文件系统，不依赖 NAS 的跨主机锁语义。

部分订阅成功：仅用本次成功或有效缓存输入，按订阅顺序、原节点顺序生成候选；
不排序、不去重、不拼回失败订阅的旧节点。全部失败或筛选为空：保留当前节点和文件。
精确比较协议、地址、凭据、参数、名称和顺序，完全一致则不写文件、不替换节点、不重置健康状态。

变化时先完整构造候选，原子重命名文件成功后，在节点组写锁下完成内存切换。
写文件或校验失败，旧节点不变。文件发布与内存提交间没有可返回错误的外部操作；
若进程在这极短窗口崩溃，重启从已完整校验的新文件恢复。
输出文件请勿由旧脚本或其他进程同时写入；同一输出路径只能有一个管理实例。
更新不会关闭已交付给调用方的连接，新连接在切换后选择新列表。
相同节点对象复用健康状态；删除节点的检查任务取消、状态回调不再把它加入节点组。
普通网络健康检查在当前有限超时检查结束后退出，休眠立即取消；file 检查取消其子进程。
LHA 当前选择、Web 状态和节点域名直连路由同步更新；历史记录保留，流量仍按来源 IP 累计。
单个 main 动态节点也检查健康，方便后续 1↔N 更新；普通静态单节点仍不做健康检查。

## N1 迁移（先只试 asia）

1. 下载正式 Release 中的 Linux ARM64 tar.gz，或需要排查时下载开发分支 Actions artifact。
   正式包使用 Release 附带的 checksums.txt 核对压缩包；Actions 测试包则用 SOURCE_COMMIT 和 SHA256SUMS 核对。
   先备份原二进制、四份主配置、四份节点文件、旧脚本和 crontab，不删除它们。
2. 检查现有 systemd 的 ExecStart 和配置文件路径，按真实路径安装新二进制。
   文件替换用临时文件加 mv，保留可执行权限。升级二进制及改主配置后首次切换需要管理员重启服务；
   后续订阅刷新不重启，Glider 本身不会执行 systemctl。
3. 安装 common 示例，填真实订阅；安装/合并 asia 示例，保留原规则、DNS、监听与 Web 设置。
   把原 `include=glider_db_asia_ssr.conf` 删除，换成示例的 subscription-output。
   保留旧节点文件作为启动快照，`chmod 600` 所有含凭据配置、输出文件。
4. 在旧 crontab 中先注释 `glider_subscribe.sh asia` 那一条，避免双方同时管理 asia；
   暂时保留其余三个地区任务，然后启动新 asia 实例。
5. 观察日志：先 restored（若有旧文件），随后 cache hit/refreshed、applied 或 content unchanged。
   用实际网页和 `curl --socks5-hostname 127.0.0.1:7575 https://example.com/` 验证转发。
   对照 Web 的 main 节点名称、数量、LHA 选择和文件中的 forward 条目。
6. 手动刷新：`systemctl kill -s HUP glider`。HUP 触发同一串行刷新器，仍遵守缓存 TTL；
   不强制绕过有效缓存。不要用 `systemctl reload`，除非你的 unit 已配置对应 ExecReload。
   检查未变刷新后输出 mtime 不变、节点健康状态不重置。
7. 保持一个持续下载/长连接，再触发真实节点变化，观察旧连接与新连接。
   测试订阅不可达时旧节点保留、重启仍从输出快照恢复；不要贴出含凭据的文件/API。
8. asia 通过后，依次安装 hk/jp/us 示例，分别注释原对应 cron，再切换对应服务。
   缓存目录四实例相同，输出路径各不相同。刷新从每轮完成后按间隔运行，不再对应旧 cron 墙上时间。

若要验证本机代理路径，用可控测试订阅和独立测试实例调整路径；不要破坏生产配置来制造失败。
配置修改需要重启，修改筛选条件时有效原始缓存可直接复用。

## 回退

1. 停止待回退实例，恢复原二进制和备份主配置。
2. 删除原生 subscription 字段/公共订阅 include，恢复原静态 `include=glider_db_<区域>_ssr.conf`。
   可以用备份节点文件，或用本版生成的标准 forward 文件继续作为静态 include。
3. 启动原服务，确认静态代理恢复，再恢复该区域旧脚本 cron；
   确认原生订阅已经关闭后才允许旧脚本重新管理同一节点文件。
4. 缓存可以保留以便再次测试，也可以在原生进程停止后删除；不要删掉唯一可用的节点快照。

## 验证边界

自动测试使用虚构节点、本机 HTTP、SOCKS 代理和 TCP echo，不访问真实订阅。
覆盖 SS/SSR/编码特殊字符、订阅整份拒绝、多订阅部分成功、空候选、缓存 TTL、不滑动有效期、
四独立进程缓存锁、失败保留、响应大小限制、GET/逐订阅后备/远端 DNS、取消、
运行时替换、精确相同不提交、重复节点/顺序、持久化失败、旧 TCP 连接延续、新连接路由、
检查任务退出及并发选择/Web 快照。Actions 运行全套 race tests、vet 和 Linux ARM64/Windows 交叉编译。

源码及本地/CI 测试不能代替 N1 验证：真实订阅服务、PC/本机实际六条路径、SS/SSR 服务端互通、
ARM64 运行表现、systemd 配置以及实网长连接连续性均由 N1 实机确认。v0.22.0 已在 N1 完成用户实测后发布；其他架构的交叉编译不等同于实机验证。

## 诊断版：跟随 -verbose

无需新增开关，原命令加 `-verbose` 即可。通过真实订阅 GET 检查 7373，
日志列出路径编号及代理 IP/端口、HTTP 状态、已知响应类型、字节数、
明文/Base64/HTML/JSON-like 类型、SS/SSR/其他行计数，以及解析失败阶段和行号。
不会打印响应正文、原始 URL、重定向地址、token、密码或任何节点参数值。
错误仅输出固定类别，网络错误区分超时、DNS、连接拒绝及部分 TLS 原因；
无法安全细分的代理错误统一标记 network-proxy-or-tls-failed。
解析规则见下方混合订阅说明；格式损坏的 SS/SSR 仍导致整份订阅失败。

例如在确认 systemd 原实例已停止、端口未被占用后：
```sh
/usr/local/sbin/glider -config /root/cfg/glider.conf -verbose
```
若保留旧快照可正常服务，保留它即可；快照恢复不会阻止后台订阅 GET。
有效缓存命中不会联网，排查真实 GET 时可临时设置 subscription-cache-ttl=0，
完成后恢复 3600。不要同时启动两个使用相同监听端口/输出文件的进程。

缓存默认改为 `/tmp/.subscription-cache`（使用系统临时目录，Windows 对应 TEMP），
显式 subscription-cache-dir 始终优先；已有设置 /tmp/.subscription-cache 可直接继续使用。
LHA history 新增 `lha-history-dir=/tmp/.glider-history`，下面仍按实例 ID 分目录隔离；
相对路径相对于主配置目录。兼容旧环境变量 GLIDER_LHA_HISTORY_PATH，其非空时优先，
并直接指定完整实例目录，因此四实例使用该环境变量时必须各自指定不同目录。
不会自动搬移或删除 cfg 下原有 history；如需旧历史，停服务后将原目录复制到新位置，
或显式 lha-history-dir 指回旧基目录。/tmp 重启可能清空，持久历史请选择其他目录。
节点输出文件保持原 subscription-output 路径，独立于临时缓存/history。

## 混合订阅中的非节点行

Base64 解码后和逐行明文输入均允许 UTF-8 BOM、空行、以 # / ; / // 开头的注释，
以及 MAX=无符号十进制整数、STATUS=说明、REMARKS=说明 元信息行。这些行不是节点，不参加筛选。
v0.23.0 转换 SS、SSR、AnyTLS、Trojan 和受支持的 VLESS 配置；
VMess、Hysteria/Hysteria2/hy2、TUIC、WireGuard、Snell 等已识别但不支持的分享链接明确计数跳过。
支持节点的顺序及重复项保留。传输与参数限制见[协议适配说明](subscription-protocols.md)。
未知文本、未知 scheme、非法 MAX 数字、损坏节点仍整体拒绝，不接纳半份结果。
仅含元信息或不支持协议、没有有效受支持节点的输入，不作为有效节点缓存。
协议或区域筛选后没有候选时，不覆盖当前节点与输出快照。

-verbose 的 body 摘要新增 classes= 固定类别计数，包括 comment、metadata-max/status/remarks、
unsupported-vmess 等；无法识别内容只标记 unknown-text 或 unknown-scheme，不输出实际文本或 scheme。
解析错误附带 class=，无需为了诊断打印原行。
下载过程另外区分 proxy-dial、tls-start/ok/failed 和 response-first-byte；
EOF、流截断、网络不可达增加安全错误分类。由此可继续排查没有 HTTP 响应的订阅。
HTTP Content-Type 仅作为诊断信息：某些服务会用 text/html 返回有效 Base64 订阅，
程序按正文格式解析，不单凭该响应头认定是网页。

N1 日志已证明 7373 能为其中两个订阅返回完整 Base64 正文；
其“其他行”的具体内容仍未知，本次只兼容以上明确类别，不能据此宣称真实订阅已修复。

## 订阅标题、失败重试与 Status

不改变 subscription=URL 原写法。在公共配置中可按顺序配标题：

```ini
subscription-title=主力订阅
subscription=https://subscription.example.invalid/REPLACE_TOKEN_1
subscription-title=备用订阅
subscription=https://subscription.example.invalid/REPLACE_TOKEN_2
subscription-interval=3600
subscription-retry-interval=7200
```

标题是独立列表，按 subscription 的最终列表顺序对应；include/CLI 中的追加规则不变。
可完全不配置标题，此时显示“订阅 1、订阅 2”；配置时必须每个订阅都有一条 title，
允许空 title= 使用默认编号。标题错配数量启动报错，避免页面误标订阅。
改变标题不改变 URL 缓存键、节点顺序或节点名称。

此前仅在启动、正常周期到达或收到 HUP 时刷新；现在部分或全部订阅失败会额外重试，
默认等待 7200 秒（2 小时，从本轮刷新完成算起），可配置。成功输入继续复用有效缓存，
失败输入重新走下载后备；不会强制重复下载成功且仍有效的订阅。
本轮全部输入成功且应用完成后恢复正常周期。失败时严格等待 retry-interval，不被较短的正常周期提前唤醒；建议保持至少 7200 秒，避免频繁请求。
0 关闭额外重试。有效输入但筛选为空仍保留旧节点，按正常周期处理，不按网络故障反复下载。
所有刷新仍串行，HUP 可提前唤醒等待，退出取消等待和下载。
本版刷新间隔从每轮完成后计时，避免慢下载导致紧接着连续触发。

Status 页面新增 Subscriptions 区域，每项显示标题、脱敏 URL、稳定短 ID、最近结果、
检查时间和上次成功下载时间，还显示下一次刷新时间及节点应用结果。
- 成功：该订阅本次下载并完整解析成功，或有有效缓存可用。筛选命中 0 个节点也会明确显示 0。
- 失败：该订阅本次无法取得完整可用输入；其他成功订阅独立显示，保留此前上次成功时间。
- 刷新中：显示正在处理及上一轮结果；页面不会等待网络请求完成才返回。
- 有效缓存：检查时间更新，上次成功下载时间保持缓存原来的下载时间，不冒充新下载。
- 页面整体区分节点已更新、内容未变、筛选为空保留旧节点、所有输入失败和文件写入失败。

URL 的 userinfo、整个路径、查询参数及 fragment 都隐藏，仅保留协议和主机用于辨识，
因为 token 也可能在路径中。相同主机可通过标题和 ID 区分；页面/API 均不返回完整订阅 URL。
所有标题和 URL 作为文本显示，不作为 HTML 或可执行链接。
页面每 5 秒默认刷新；页面 Refresh now 按钮只刷新显示，不额外发起订阅 GET，手动订阅刷新仍用 HUP。

最近检查结果是本进程状态，不另写 cfg 或新的持久化状态文件；重启先显示等待，随后启动刷新更新。
有效缓存的原始成功下载时间仍从共享缓存恢复。缓存过期后失败不把旧输入当作成功恢复。
无订阅配置时不显示此区块，静态配置的 Web 行为保持原样。
