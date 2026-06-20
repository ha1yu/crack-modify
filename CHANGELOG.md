# Changelog

本项目所有重要变更都会记录在此文件中。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

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
