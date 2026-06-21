# Changelog

本项目所有重要变更都会记录在此文件中。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

## [v1.3.0] - 2026-06-22 ⚠️ 破坏性变更

相对 [v1.2.0](#v120---2026-06-21)，**消除目标输入的歧义**：协议完全由 `-m` 决定，不再支持 `|协议` 后缀，也不再按端口自动识别。

### ⚠️ 破坏性变更（Breaking）

- **废弃 `ip:port|协议` 语法**：`-i '127.0.0.1:3307|mysql'` 不再支持（含 `|` 的端口会被当非法跳过）。协议改由 `-m` 指定。
- **废弃 `-m all`**：`-m` 现在必填具体协议（mysql/ssh/...）。原来一次 `-m all` 跑多协议混合目标，现在需分多次（每次一个 `-m`）。
- **废弃端口→协议自动识别**：不再有"3306 自动识别为 mysql"的行为。目标的协议完全由 `-m` 决定，**端口任意**（mysql 在 3307、65535 都行，只要 `-m mysql`）。
- **迁移指南**：
  - `crack-modify -i '127.0.0.1:3307|mysql'` → `crack-modify -i 127.0.0.1:3307 -m mysql`
  - `crack-modify -f targets.txt -m all` → `crack-modify -f targets.txt -m mysql`（再 `-m ssh` 各跑一次）
  - `crack-modify -i 127.0.0.1:3306`（原靠端口识别）→ `crack-modify -i 127.0.0.1:3306 -m mysql`（必须显式 -m）

### 变更

- `pkg/crack/parse.go`：`ParseTargets` 只解析 `ip:port`（含 CIDR/段/逗号展开），不再切 `|协议`、不再查端口表；目标 `Protocol` 字段留空，由调用方按 `-m` 填充。删除 `FilterModule`（不再需要过滤）。
- `pkg/crack/config.go`：删除 `PortNames`（端口映射）与 `SupportProtocols`（旧 map），新增导出的 `SupportedProtocols` 切片 + `IsSupportedProtocol()` 供 `-m` 合法性校验。
- `cmd/root.go`：`-m` 默认值改为空（**必填**），`validateOptions` 校验 `-m` 非空且合法；`run()` 里给每个解析出的目标填上 `-m` 指定的协议（替代原 `FilterModule`）。
- 测试同步更新：`crack_test.go`（`ParseTargets` 用例改为协议留空、删 `TestFilterModule`、新增 `TestSupportedProtocols`）；`integration_test.go`（去 `|mysql`/`-m all`、删 `TestCLICrackModuleFilter`、新增 `-m` 必填/非法/旧语法废弃 3 个测试）。
- **新增 `crack.html` 命令配置页**：纯前端单文件（双击浏览器打开），13 协议场景预设按钮、表单勾选实时生成命令、一键复制。
  - module 下拉默认选中 `ssh`；去掉 `all` 选项；`-i` 框 placeholder 与帮助卡片移除 `|协议` 与端口映射表，说明改为"协议由 `-m` 决定，端口任意"。
- README 示例与 help 输出块同步更新。

### 为什么这样改

原设计里"协议"有三个来源（`-m` 过滤、`|协议` 标注、端口识别），语义重叠且易混淆（见用户反馈："-m mysql 是不是就不用 |mysql 了"）。现统一为**单一来源 `-m`**，目标输入只剩 `ip:port`，彻底无歧义。

### 验证

- `go vet ./...` 干净；`go test ./...` 4 包全绿、103 用例全 PASS。
- `go test -race` 无数据竞争。
- 远程跨公网测试（mysql/ssh/redis 等真实服务）+ `-m` 必填/非法协议/旧 `|协议` 废弃 全部符合预期。

## [v1.2.0] - 2026-06-21

相对 [v1.1.0](#v110---2026-06-21)，简化 CLI 结构（去掉子命令层）、修正退出码行为、加固 Docker 测试稳定性。

### 变更（CLI 重构）

- **去掉 `crack` 子命令**：程序只有一个爆破功能，无需子命令嵌套。原 `crack-modify crack -i ...` 简化为 `crack-modify -i ...`，`-h` 直接显示全部 flag。删除 `cmd/crack.go`，选项/flag/爆破逻辑合并进 `cmd/root.go` 的根命令。
- **根命令直接执行爆破**：`RunE` 直接跑爆破流程，`PersistentPreRunE` 统一做选项校验/加载。
- **退出码行为修正**：错误处理从 `gologger.Fatal` 改为返回 `error`（让 cobra 统一打印）。无输入 / 文件不存在 → 退出码非零；正常爆破完成 → 退出码 0；`运行时间` 仅在真正执行爆破后打印。
- **flag 说明统一中文**：所有 flag 帮助文本由英文改为中文，与命令描述风格一致。

### 变更（测试）

- **Docker 测试端口修复**：原固定端口（13306/16379/21211/10445 等）在 Docker Desktop 异常退出后会留下残留端口绑定，导致端口转发失效、测试 flaky。改用 30000+ 高段端口（33306/36379/31211 等），从根本上消除与系统/残留端口的冲突。
- CLI 集成测试同步去掉 `crack` 子命令参数、断言中文错误信息。

### 验证

- `go vet` 干净；`go test ./...` 4 包全绿、103 用例全 PASS。
- `go test -race` 无数据竞争。
- 本地 Docker 10 协议真实服务回归全过（120s）。
- 远程跨公网 7 协议（mysql/redis/postgres/memcached/mongodb/ssh/ftp）真实服务爆破 + CIDR/喷洒/JSON 导出全部命中。

## [v1.1.0] - 2026-06-21

相对 [v1.0.0](#v100---2026-06-20)，新增多目标输入（CIDR/IP段）与密码喷洒模式。

### 新增（功能）

- **多目标输入（CIDR / IP 段 / 逗号列表）**：目标 IP 现支持 `192.168.1.0/24`（CIDR，限制 ≤ /24 防误爆）、`192.168.1.1-128`（IP 段）、`10.0.0.1,10.0.0.2`（逗号列表），与端口/协议格式组合如 `192.168.1.0/24:445|smb`。新增 `internal/utils/ip.go` 的 `ExpandIPs`。
- **密码喷洒模式（`--spray`）**：默认字典爆破为"每用户跑全口令"，Windows 域（SMB/MSSQL）下易触发账户锁定。`--spray` 改为"每口令遍历所有用户再换下一个"，配合 `--delay` 降低单用户锁定风险。`Options.Spray` 控制任务生成顺序。

### 新增（测试）

- `internal/utils/ip_test.go`：CIDR/段/逗号列表展开的完整单元测试（含 /24、/30、超大 CIDR 拒绝、非法段等）。
- `pkg/crack/crack_test.go`：`ParseTargets` 新增 CIDR/段/逗号/超大CIDR 跳过等用例。
- `pkg/crack/runner_engine_test.go`：`TestSprayTaskOrder` 验证喷洒模式与默认模式的任务顺序差异。

## [v1.0.0] - 2026-06-20

首个正式发布版本。相对 [v0.1.0](#v010---2026-06-20)，包含工具链降级（支持 Win7）、引擎 4 项修复、完整测试体系（含 Docker 真实服务集成测试）。

### 变更（工具链降级，支持 Windows 7）

- **Go 工具链由 go1.24 降级到 `go1.20.14`**：Go 1.21+ 已放弃 Windows 7 支持，降级到 1.20.x 是产出 Win7 兼容二进制的必要条件。
- `go.mod` 的 `go` 指令由 `1.24.0` 改为 `1.20`。
- **`replace` 锁定 `golang.org/x/text` → v0.14.0、`golang.org/x/sys` → v0.6.0**：原 `go mod tidy` 曾把 x/text 升到 v0.33.0（其源码要求 go 1.24，go1.20.14 编译报 "cannot compile Go 1.24 code"）。用 `replace` 强制锁定到兼容 go1.20 的版本，绕过 MVS 自动抬升。wmiexec 的 unicode 编码功能不受影响（Docker 回归验证通过）。
- **新增 `build.sh` 多平台交叉构建脚本**：9 个目标（darwin/linux/windows × arm64/amd64/386 等），`CGO_ENABLED=0` 纯静态，`-trimpath -ldflags "-s -w"` 瘦身。
- 产物二进制内嵌 `go1.20.14`、Linux 为纯静态 ELF，Win7 兼容性有保障。

### 变更（引擎优化与修复）

针对爆破引擎 `pkg/crack/runner.go` 的 4 项正确性/性能修复：

- **B1（健壮性，高危）**：`NewRunner` 对 `Threads`/`Timeout`/`Delay` 做边界兜底。原 `--threads 0` 会让 `taskChan` 容量为 0 且不启动任何 worker，导致生产者入队永久死锁；`--timeout 0` 使 `net.DialTimeout(...,0)` 行为未定义。现：`Threads<1→1`、`Threads>1000→1000`、`Timeout<1→10`、`Delay<0→0`。
- **P1（限速语义，安全相关）**：`Delay` 限速改用全局 `time.Ticker` gate。原实现在每个 worker 内 `time.Sleep(Delay)`，多 worker 并行下实际请求速率 = `Threads/Delay`（如 `--threads 10 --delay 2` 用户以为 2 秒/次，实际 5 次/秒），限速形同虚设。现所有 worker 共享一个 ticker，请求前取令牌，整体速率严格限制为 `1 req / Delay 秒`，符合"请求间隔"语义。**注意：这是预期行为变化，`Delay>0` 时总耗时会变长（即正确的限速效果）。**
- **P3（性能）**：任务去重去掉每任务一次的 `MD5(Sprintf(...))`，改用 `map[string]struct{}` 以 `user+"\x00"+pass` 为 key（去重无需密码学强度）；`stopMap` 的 key 也从每任务 `Md5(addrStr)` 改为直接用 `addrStr`（整个 `Crack` 内不变，无需重复计算）。大字典场景（万元组）开销显著下降。
- **B5（体验）**：进度条 `bar.Increment()` 从生产者入队循环移到 worker 完成处（含被 `stopMap` 跳过的任务）。原进度条到 100% 只表示"全部已入队"，不代表完成；现反映真实完成进度。

### 新增（测试）

- **L6 Docker 真实服务集成测试** `pkg/crack/plugins/docker_test.go`：通过环境变量 `CRACK_DOCKER_TEST=1` 显式开启（默认与 `-short` 模式均跳过），对 10 个协议的真实容器验证 success/fail/error 三态：`mysql`、`postgres`、`mssql`、`redis`、`memcached`、`ftp`、`ssh`、`mongodb`(3.6 兼容 mgo)、`oracle`(XE)、`smb`。`wmi`/`wmihash`/`rdp` 因需 Windows 目标未纳入。
  - 通用 `startContainer` 辅助：随机命名、`-p 127.0.0.1:host:guest` 端口映射、`t.Cleanup` 自动 `docker rm -f`、就绪探测（TCP / 协议握手）+ 90s 超时。
  - `TestDockerErrorPath` 固化各协议对"连接被拒"的实际返回码（mysql/memcached/ftp→`CrackError`，postgres/mssql/redis→`CrackFail`，因上游仅对含 "timeout" 的错误判 `CrackError`），作为行为回归基线。
- 建立零外部依赖的测试体系，`go test ./...` 在无 mysql/redis/ssh 等真实服务的环境下稳定通过。
- **L1 纯函数单元测试** `internal/utils/utils_test.go`：`RemoveDuplicate`（含空串过滤边界）、`Md5`（已知向量）、`HasStr` / `SuffixStr` / `HasInt`。
- **L1 引擎单元测试** `pkg/crack/crack_test.go`：`ParseTargets` 表驱动覆盖（`ip:port` / `ip:port|proto` / 各类非法输入跳过 / TrimSpace）、`FilterModule`、`NewRunner` 默认字典注入与显式字典保留、`PortNames` ↔ `SupportProtocols` 一致性、内置字典健全性。
- **L2 并发引擎测试** `pkg/crack/runner_engine_test.go`：通过临时改写 `plugins.ScanFuncMap` 注入 mock ScanFunc，确定性验证 `CheckAlive`、命中即停（`CrackAll:false`）、`CrackAll:true` 继续、`CrackError` 停止目标、`Delay` 限速、`{user}` 模板 + MD5 任务去重、多目标独立计数。
- **L3 插件契约测试** `pkg/crack/plugins/plugins_test.go`：13 协议注册完整性、返回码常量值、`Service` 结构、各插件对不可达目标调用不 panic 且返回合法码。
- **L4 mock-TCP 插件测试** `pkg/crack/plugins/plugins_mock_test.go`：用 Go 原生 `net.Listen` 搭 mock 服务，验证 memcached / mongodb 未授权探测的真实命中、未命中回退与连接错误路径。
- **L5 CLI 集成测试** `cmd/crack_integration_test.go`：编译真实二进制 `exec` 运行，端到端验证根/crack `-h`、无输入报错、`--user-file` / `-f` 不存在报错、死端口不崩溃、混合格式目标解析、`-m` 过滤、`--result` JSON 导出。
- 支持 `-short` 跳过 CLI 集成测试与网络冒烟，便于 CI。

### 变更（测试相关）

- 测试设计原则：全部用 mock ScanFunc 与本地 TCP mock，不纳入需要真实服务的 success/fail 用例（避免 flaky）。
- 通过 `-race` 验证爆破引擎工作池无数据竞争。

### 验证结果

- `go vet ./...` 通过（仅余上游原样复制 `smb/session.go` 的 2 处既有 vet 警告，非本次引入）。
- `go test ./... -count=1`（不含 Docker 可选测试）：68 个子用例全部 PASS，4 包全 `ok`。
- `go test -race`：无数据竞争。
- 覆盖率：`pkg/crack` 97.3% / `plugins` 51.1% / `internal/utils` 40% / `cmd` 10.6%。
- Docker 集成（`CRACK_DOCKER_TEST=1`）：10 个协议真实容器 success/fail/error 全部 PASS，默认与 `-short` 模式下 11 个 Docker 用例均 skip，不影响 CI。

## [v0.1.0] - 2026-06-20

首个可用版本。从 [zpscan](https://github.com/niudaii/zpscan) `v1.8.39` 剥离 `crack`
模块，作为独立、可单独编译的命令行工具，移除了对其余扫描模块及 `config.yaml` 的依赖。

### 新增

- **crack 子命令**：常见服务弱口令爆破，支持 13 种协议
  `ftp, ssh, wmi, wmihash, smb, mssql, oracle, mysql, rdp, postgres, redis, memcached, mongodb`，
  其中 `memcached` / `mongodb` 支持未授权检测。
- **目标解析**：同时支持按端口自动识别协议（`127.0.0.1:3306`）和显式指定协议
  （`127.0.0.1:3307|mysql`）两种输入格式，单条 `-i` 与文件 `-f` 输入均可。
- **爆破引擎**：存活探测（TCP）→ 协议爆破两阶段；多线程工作池、`{user}` 模板口令替换、
  任务 MD5 去重、命中即停（`--crack-all` 可继续爆破该目标的全部口令）、`--delay` 限速、
  进度条（`cheggaaa/pb`）、`concurrent-map` 早停控制。
- **内置字典**：`pkg/crack/config.go` 内置默认 `userMap`（各协议默认用户名）、
  `templatePass`（21 条 `{user}` 模板口令）、`commonPass`（约 80 条常用口令），
  无需外部 `config.yaml` / `resource.zip`。
- **自定义字典**：支持 `--user` / `--pass`（逗号分隔）与 `--user-file` / `--pass-file`。
- **协议插件契约**：`ScanFunc` 函数注册表，统一返回 `CrackError`（连接异常→停止目标）/
  `CrackFail`（口令错误→继续）/ `CrackSuccess`（命中）。
- **内嵌协议客户端**：`grdp`（RDP）、`smb`（SMB2/3）、`wmiexec`（WMI/DCOM）。
- **CLI 与输出**：基于 `cobra` 的子命令与全局 flag；`gologger` 彩色日志，`-o` 写日志与结果、
  `--result` 单独导出命中结果 JSON，`--debug` / `--no-color` 可选。
- 文档：`README.md` 使用说明与示例，`CHANGELOG.md` 变更记录。

### 变更（相对上游 zpscan）

- 移除 `config` 包及运行时读取 `config.yaml` 的逻辑；字典改由 `crack.NewRunner` 在为空时
  自动注入内置默认值。
- 移除与 crack 无关的扫描模块（`webscan` / `ipscan` / `dirscan` / `domainscan` / `pocscan`）
  及 `internal/utils` 中 crack 未使用的辅助函数。
- 模块路径由 `github.com/niudaii/zpscan` 改为 `crack-modify`。
- `go.mod` 仅保留 crack 实际依赖及两条必要 `replace`（`gologger`→`niudaii`、`grdp`→`shadow1ng`）。

### 技术栈

- Go 1.18+（`go mod tidy` 解析后实际为 `go 1.24`）
- `github.com/spf13/cobra`、`github.com/projectdiscovery/gologger`、`github.com/cheggaaa/pb/v3`、
  `github.com/orcaman/concurrent-map/v2`
- 各协议驱动：`jlaffaye/ftp`、`golang.org/x/crypto/ssh`、`go-sql-driver/mysql`、
  `microsoft/go-mssqldb`、`sijms/go-ora/v2`、`lib/pq`、`go-redis/redis`、`gopkg.in/mgo.v2`、
  `stacktitan/smb`、`C-Sto/goWMIExec`、`tomatome/grdp`(→`shadow1ng/grdp`)
