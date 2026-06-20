# Changelog

本项目所有重要变更都会记录在此文件中。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

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
