# 原生 SS/SSR 订阅：N1 测试版

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
| subscription | 无 | 可重复；按声明顺序合并，各订阅独立尝试下载路径 |
| subscription-region | 空 | 区域说明标签，不推断地理位置；筛选取决于 match/exclude |
| subscription-match | 空 | 可重复；大小写敏感字面子串 OR 匹配，空列表接受所有节点 |
| subscription-exclude | 遊戲 | 可重复；任意非空关键词命中就排除；显式设置替代默认，可用空值关闭排除 |
| subscription-output | 必填 | 本实例唯一的生成文件，同时是最后成功应用节点快照 |
| subscription-proxy | direct:// | 可重复；显式配置后替代默认列表，不隐式添加直连 |
| subscription-cache-dir | subscription-cache | 四实例应设置为同一绝对目录 |
| subscription-cache-ttl | 3600 | 原始订阅新鲜度秒数；0 表示每次刷新都下载 |
| subscription-interval | 3600 | 后台刷新周期秒数，与 TTL 独立，必须大于 0 |
| subscription-connect-timeout | 3 | 每条路径底层 TCP 连接超时秒数，必须大于 0 |
| subscription-timeout | 15 | 每条路径整次 GET 总超时秒数，包含代理握手、TLS、响应读取 |
| subscription-max-bytes | 8388608 | 解压后响应大小上限；超过整份拒绝 |

SSR 用 `group + 空格 + remarks` 匹配，SS 用片段名称匹配。关键词不拆分逗号、
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
只支持 SS/SSR，不转换 VMess/VLESS，不导出 JSON。

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

1. 下载开发分支 Actions 的 ARM64 artifact，解压外层 ZIP 和内部 tar.gz。
   `SOURCE_COMMIT` 标出源码，`sha256sum -c SHA256SUMS` 核对 glider。
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
   缓存目录四实例相同，输出路径各不相同。刷新定时按启动时间间隔运行，不再对应旧 cron 墙上时间。

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
ARM64 运行表现、systemd 配置以及实网长连接连续性均由 N1 实机确认。此分支不发布正式 Release。
