# crack-modify

常见服务弱口令爆破工具，从 [zpscan](https://github.com/niudaii/zpscan) 的 `crack` 模块剥离而来，作为独立可编译的命令行工具，去掉了对其余模块（webscan / ipscan / dirscan / domainscan / pocscan）以及 `config.yaml` 的依赖。

> 当前版本：**v0.1.0**　|　变更记录见 [CHANGELOG.md](./CHANGELOG.md)

## 功能

- 支持默认端口协议和自定义协议爆破：`127.0.0.1:3306`（按端口识别 mysql）、`127.0.0.1:3307|mysql`（显式指定协议）
- 支持常见服务口令爆破与未授权检测：`ftp, ssh, wmi, wmihash, smb, mssql, oracle, mysql, rdp, postgres, redis, memcached, mongodb`
- 内置默认用户名 / 模板口令 / 常用口令字典，也支持 `--user/--pass/--user-file/--pass-file` 自定义
- 存活探测 → 协议爆破，多线程 + 命中即停（`--crack-all` 可爆破全部），支持 `--delay` 限速

## 使用

```
crack-modify crack -h
常见服务弱口令爆破,支持ftp,ssh,wmi,wmihash,smb,mssql,oracle,mysql,rdp,postgres,redis,memcached,mongodb

Flags:
      --crack-all          crack all user:pass
      --delay int          delay between requests in seconds (0 to disable)
  -m, --module string      choose one module to crack(ftp,ssh,wmi,wmihash,smb,mssql,oracle,mysql,rdp,postgres,redis,memcached,mongodb) (default "all")
      --pass string        pass(example: --pass 'admin,root')
      --pass-file string   pass file(example: --pass-file 'pass.txt')
      --threads int        number of threads (default 1)
      --timeout int        timeout in seconds (default 10)
      --user string        user(example: --user 'admin,root')
      --user-file string   user file(example: --user-file 'user.txt')

Global Flags:
      --debug               show debug output
  -i, --input string        single input(example: -i 'xxx')
  -f, --input-file string   inputs file(example: -f 'xxx.txt')
      --no-color            disable colors in output
  -o, --output string       output file to write log and results (default "result.txt")
      --result string       output file to write found results
```

### 示例

```bash
# 编译
go build -o crack-modify .

# 单个目标（按端口识别协议）
./crack-modify crack -i 127.0.0.1:3306 -m mysql --threads 4 --timeout 5

# 指定协议（非默认端口）
./crack-modify crack -i '127.0.0.1:3307|mysql' -m mysql

# 目标文件，混合两种格式
./crack-modify crack -f targets.txt -m all --threads 10

# 自定义字典，命中后继续爆破该目标的所有口令，结果写入 JSON
./crack-modify crack -f targets.txt --user-file user.txt --pass-file pass.txt --crack-all --result found.json
```

`targets.txt` 每行一个目标，支持：

```
127.0.0.1:3306
127.0.0.1:3307|mysql
127.0.0.1:6379
192.168.1.10:22
```

## 目录结构

```
crack-modify/
├── go.mod, go.sum, main.go
├── cmd/                       # cobra CLI（root.go + crack.go）
│   └── crack_integration_test.go               # 端到端 CLI 集成测试
├── internal/utils/            # Md5 / RemoveDuplicate / ReadLines / FileExists / SaveMarshal
│   └── utils_test.go                            # 纯函数单元测试
└── pkg/crack/
    ├── config.go parse.go check.go runner.go   # 端口/协议映射、目标解析、存活检测、爆破引擎
    ├── crack_test.go runner_engine_test.go      # 解析/引擎/并发单元测试
    └── plugins/                                 # 13 个协议插件 + 内嵌 grdp / smb / wmiexec 协议客户端
        ├── plugins_test.go                      # 插件注册表与契约测试
        └── plugins_mock_test.go                 # 基于 mock TCP 的插件行为测试
```

## 测试

测试遵循**零外部依赖**原则：不依赖真实 mysql/redis/ssh 等服务，全部用 mock ScanFunc 与本地 `net.Listen` mock 服务，`go test ./...` 在干净环境一次通过。

```bash
# 运行全部测试（68 个子用例）
go test ./... -count=1

# 带竞态检测（验证爆破引擎工作池无数据竞争）
go test ./pkg/crack/ ./pkg/crack/plugins/ -race -count=1

# 覆盖率
go test ./... -count=1 -cover

# 快速模式（跳过 CLI 集成测试与网络冒烟，适合 CI）
go test ./... -count=1 -short
```

测试分层：

| 层级 | 文件 | 覆盖内容 |
|---|---|---|
| L1 纯函数单元 | `internal/utils/utils_test.go` | `RemoveDuplicate` / `Md5` / `HasStr` 等 |
| L1 引擎单元 | `pkg/crack/crack_test.go` | `ParseTargets` / `FilterModule` / `NewRunner` 默认注入 / 协议注册表一致性 |
| L2 并发引擎 | `pkg/crack/runner_engine_test.go` | `CheckAlive` / 命中即停 / `CrackAll` / `CrackError` 停止 / `Delay` 限速 / 任务去重 |
| L3 插件契约 | `pkg/crack/plugins/plugins_test.go` | 13 协议注册完整性 / 返回码常量 / 可调用不 panic |
| L4 mock TCP | `pkg/crack/plugins/plugins_mock_test.go` | memcached / mongodb 未授权探测的真实行为 |
| L5 CLI 集成 | `cmd/crack_integration_test.go` | 编译真实二进制，端到端验证 `-h`、参数校验、目标解析、`--result` 导出 |

当前覆盖率：`pkg/crack` 97.3%、`plugins` 51.1%、`internal/utils` 40%、`cmd` 10.6%（CLI 分支较多，以集成测试保证行为）。

> 注：需要真实数据库/SSH 服务的 success/fail 用例（如上游 `plugins_test.go`）未纳入，因其无法在无外部服务的 CI 环境稳定运行。

## 说明

- 不再需要上游的 `config.yaml` 与 `resource.zip`。未指定字典时，自动使用 `pkg/crack/config.go` 内置的 `userMap` / `templatePass` / `commonPass`。
- 协议返回约定：`CrackError`（连接/网络错误，停止该目标）、`CrackFail`（口令错误，继续）、`CrackSuccess`（命中）。
