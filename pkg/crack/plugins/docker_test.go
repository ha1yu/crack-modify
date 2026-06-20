package plugins

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// 这个文件包含基于 Docker 的真实服务集成测试。
//
// 运行门槛: 默认跳过。显式开启需满足:
//   - 环境变量 CRACK_DOCKER_TEST=1 (推荐, 显式 opt-in)
//   - 或未启用 -short
//   - 且 docker daemon 可达
// 这样可保证无 Docker 的 CI 环境(`go test ./... -short`)仍 100% 绿。
//
// 运行: CRACK_DOCKER_TEST=1 go test ./pkg/crack/plugins/ -run TestDocker -count=1 -v -timeout 20m
//
// wmi/wmihash/rdp 需要 Windows 目标, 本文件不覆盖(维持 mock/不可达路径)。

// dockerEnabled 判断是否运行 Docker 集成测试。
func dockerEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv("CRACK_DOCKER_TEST") != "1" {
		t.Skip("skipping docker integration test; set CRACK_DOCKER_TEST=1 to enable")
	}
	if testing.Short() {
		t.Skip("skipping docker integration test in -short mode")
	}
	if !dockerAvailable() {
		t.Skip("docker daemon not reachable")
	}
}

// dockerAvailable 探测 docker daemon 是否可达。
func dockerAvailable() bool {
	cmd := exec.Command("docker", "info", "--format", "{{.ServerVersion}}")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return false
	}
	return strings.TrimSpace(out.String()) != ""
}

// containerSpec 描述一个待启动的容器。
type containerSpec struct {
	image      string   // 镜像
	name       string   // 容器名(自动加随机后缀避免冲突)
	env        []string // 环境变量
	hostPort   int      // 宿主端口(固定, 便于 127.0.0.1 直连)
	guestPort  int      // 容器内端口
	cmdArgs    []string // 额外启动参数
	cmd        []string // 覆盖 CMD
}

// startContainer 启动一个容器并阻塞直到 ready 返回 true 或超时。
// 失败时 t.Fatal。t.Cleanup 自动 docker rm -f。
func startContainer(t *testing.T, spec containerSpec, ready func(host string, port int) bool) {
	t.Helper()
	// 容器名加时间戳后缀, 避免残留冲突
	uniqName := fmt.Sprintf("%s-%d", spec.name, time.Now().UnixNano())

	args := []string{"run", "-d", "--rm",
		"--name", uniqName,
		"-p", fmt.Sprintf("127.0.0.1:%d:%d", spec.hostPort, spec.guestPort),
	}
	for _, e := range spec.env {
		args = append(args, "-e", e)
	}
	if len(spec.cmd) > 0 {
		args = append(args, spec.image)
		args = append(args, spec.cmd...)
	} else {
		args = append(args, spec.image)
		args = append(args, spec.cmdArgs...)
	}

	t.Logf("starting container %s (%s) port %d->%d", uniqName, spec.image, spec.hostPort, spec.guestPort)
	out, err := exec.Command("docker", args...).Output()
	if err != nil {
		// 端口可能被占, 打印日志便于诊断
		logs := containerLogs(uniqName)
		t.Fatalf("docker run failed: %v\nlogs:\n%s", err, logs)
	}
	containerID := strings.TrimSpace(string(out))

	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", containerID).Run()
	})

	// 等待就绪
	deadline := time.Now().Add(90 * time.Second) // oracle/mssql 启动慢
	for time.Now().Before(deadline) {
		if ready("127.0.0.1", spec.hostPort) {
			return
		}
		time.Sleep(2 * time.Second)
	}
	logs := containerLogs(uniqName)
	t.Fatalf("container %s did not become ready in 90s\nlogs:\n%s", uniqName, logs)
}

// containerLogs 取容器日志(截断)。
func containerLogs(name string) string {
	out, err := exec.Command("docker", "logs", "--tail", "40", name).CombinedOutput()
	if err != nil {
		return fmt.Sprintf("(failed to get logs: %v)", err)
	}
	return string(out)
}

// tcpReady 返回一个 ready 探测函数: TCP 端口可连即就绪。
func tcpReady(port int) func(string, int) bool {
	return func(host string, p int) bool {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, p), 2*time.Second)
		if err != nil {
			return false
		}
		c.Close()
		return true
	}
}

// waitTCP 在测试内同步等待一个 TCP 端口可连(用于容器外的临时探测)。
func waitTCP(t *testing.T, host string, port int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 2*time.Second)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("tcp %s:%d not reachable in %v", host, port, timeout)
}

// =====================================================================
// 协议测试: 每个协议覆盖 success / fail 两态, 部分覆盖 error。
// =====================================================================

// --- MySQL ----------------------------------------------------------------

func TestDockerMysqlCrack(t *testing.T) {
	dockerEnabled(t)
	const port = 13306
	startContainer(t, containerSpec{
		image: "mysql:5.7", name: "cm-mysql",
		hostPort: port, guestPort: 3306,
		env: []string{"MYSQL_ROOT_PASSWORD=rootpass"},
	}, func(host string, p int) bool {
		// mysql 就绪判据: 用 root/rootpass 实际尝试连接,
		// 只有返回 CrackSuccess(认证通过)或 CrackFail(1045, 说明已就绪只是口令不对)才算就绪。
		// TCP 可连不代表 mysql 已完成初始化(启动中会重置 root 口令)。
		svc := &Service{Ip: host, Port: p, Protocol: "mysql", User: "root", Pass: "rootpass", Timeout: 2}
		got, _ := MysqlCrack(svc)
		return got == CrackSuccess || got == CrackFail
	})

	svc := &Service{Ip: "127.0.0.1", Port: port, Protocol: "mysql", Timeout: 8}
	t.Run("success", func(t *testing.T) {
		svc.User, svc.Pass = "root", "rootpass"
		if got, _ := MysqlCrack(svc); got != CrackSuccess {
			t.Errorf("mysql success = %v, want CrackSuccess", got)
		}
	})
	t.Run("fail", func(t *testing.T) {
		svc.User, svc.Pass = "root", "wrongpass"
		if got, _ := MysqlCrack(svc); got != CrackFail {
			t.Errorf("mysql fail = %v, want CrackFail", got)
		}
	})
}

// --- PostgreSQL -----------------------------------------------------------

func TestDockerPostgresCrack(t *testing.T) {
	dockerEnabled(t)
	const port = 15432
	startContainer(t, containerSpec{
		image: "postgres:13", name: "cm-pg",
		hostPort: port, guestPort: 5432,
		env: []string{"POSTGRES_PASSWORD=pgpass"},
	}, tcpReady(port))
	waitTCP(t, "127.0.0.1", port, 60*time.Second)
	time.Sleep(5 * time.Second)

	svc := &Service{Ip: "127.0.0.1", Port: port, Protocol: "postgres", Timeout: 8}
	t.Run("success", func(t *testing.T) {
		svc.User, svc.Pass = "postgres", "pgpass"
		if got, _ := PostgresCrack(svc); got != CrackSuccess {
			t.Errorf("postgres success = %v, want CrackSuccess", got)
		}
	})
	t.Run("fail", func(t *testing.T) {
		svc.User, svc.Pass = "postgres", "wrongpass"
		if got, _ := PostgresCrack(svc); got != CrackFail {
			t.Errorf("postgres fail = %v, want CrackFail", got)
		}
	})
}

// --- Redis ----------------------------------------------------------------

func TestDockerRedisCrack(t *testing.T) {
	dockerEnabled(t)
	const port = 16379
	// 通过 cmd 覆盖, 启动时设置 requirepass
	startContainer(t, containerSpec{
		image: "redis:6", name: "cm-redis",
		hostPort: port, guestPort: 6379,
		cmd: []string{"redis-server", "--requirepass", "redispass"},
	}, tcpReady(port))

	svc := &Service{Ip: "127.0.0.1", Port: port, Protocol: "redis", Timeout: 5}
	t.Run("success", func(t *testing.T) {
		svc.User, svc.Pass = "", "redispass"
		if got, _ := RedisCrack(svc); got != CrackSuccess {
			t.Errorf("redis success = %v, want CrackSuccess", got)
		}
	})
	t.Run("fail", func(t *testing.T) {
		svc.User, svc.Pass = "", "wrongpass"
		if got, _ := RedisCrack(svc); got != CrackFail {
			t.Errorf("redis fail = %v, want CrackFail", got)
		}
	})
}

// --- Memcached (未授权) ----------------------------------------------------

func TestDockerMemcachedCrack(t *testing.T) {
	dockerEnabled(t)
	const port = 21211
	startContainer(t, containerSpec{
		image: "memcached:1.6", name: "cm-memcached",
		hostPort: port, guestPort: 11211,
	}, tcpReady(port))

	svc := &Service{Ip: "127.0.0.1", Port: port, Protocol: "memcached", Timeout: 5}
	// memcached 无鉴权, 正常服务即未授权命中
	if got, _ := MemcachedCrack(svc); got != CrackSuccess {
		t.Errorf("memcached unauth = %v, want CrackSuccess", got)
	}
}

// --- FTP ------------------------------------------------------------------

func TestDockerFtpCrack(t *testing.T) {
	dockerEnabled(t)
	const port = 10021
	// delfer/alpine-ftp-server: 默认用户 user/userpass, 需设 USERS 环境变量
	startContainer(t, containerSpec{
		image: "delfer/alpine-ftp-server:latest", name: "cm-ftp",
		hostPort: port, guestPort: 21,
		env: []string{"USERS=ftpuser|ftppass"},
	}, tcpReady(port))
	time.Sleep(3 * time.Second)

	svc := &Service{Ip: "127.0.0.1", Port: port, Protocol: "ftp", Timeout: 8}
	t.Run("success", func(t *testing.T) {
		svc.User, svc.Pass = "ftpuser", "ftppass"
		if got, _ := FtpCrack(svc); got != CrackSuccess {
			t.Errorf("ftp success = %v, want CrackSuccess", got)
		}
	})
	t.Run("fail", func(t *testing.T) {
		svc.User, svc.Pass = "ftpuser", "wrongpass"
		if got, _ := FtpCrack(svc); got != CrackFail {
			t.Errorf("ftp fail = %v, want CrackFail", got)
		}
	})
}

// --- SSH ------------------------------------------------------------------

func TestDockerSshCrack(t *testing.T) {
	dockerEnabled(t)
	const port = 10022
	// linuxserver/openssh-server: 需设 USER_NAME/USER_PASSWORD/PASSWORD_ACCESS
	startContainer(t, containerSpec{
		image: "linuxserver/openssh-server:latest", name: "cm-ssh",
		hostPort: port, guestPort: 2222,
		env: []string{
			"USER_NAME=sshuser",
			"USER_PASSWORD=sshpass",
			"PASSWORD_ACCESS=true",
			"PUID=1000",
			"PGID=1000",
		},
	}, tcpReady(port))
	time.Sleep(8 * time.Second) // 等 sshd 起来并允许密码认证

	svc := &Service{Ip: "127.0.0.1", Port: port, Protocol: "ssh", Timeout: 8}
	t.Run("success", func(t *testing.T) {
		svc.User, svc.Pass = "sshuser", "sshpass"
		if got, _ := SshCrack(svc); got != CrackSuccess {
			t.Errorf("ssh success = %v, want CrackSuccess", got)
		}
	})
	t.Run("fail", func(t *testing.T) {
		svc.User, svc.Pass = "sshuser", "wrongpass"
		if got, _ := SshCrack(svc); got != CrackFail {
			t.Errorf("ssh fail = %v, want CrackFail", got)
		}
	})
}

// --- MSSQL ----------------------------------------------------------------

func TestDockerMssqlCrack(t *testing.T) {
	dockerEnabled(t)
	const port = 11433
	// SA 密码需满足强密码策略(大小写+数字+符号)
	startContainer(t, containerSpec{
		image: "mcr.microsoft.com/mssql/server:2019-latest", name: "cm-mssql",
		hostPort: port, guestPort: 1433,
		env: []string{
			"ACCEPT_EULA=Y",
			"MSSQL_SA_PASSWORD=SQLpass-123",
		},
	}, tcpReady(port))
	time.Sleep(15 * time.Second) // mssql 启动较慢

	svc := &Service{Ip: "127.0.0.1", Port: port, Protocol: "mssql", Timeout: 10}
	t.Run("success", func(t *testing.T) {
		svc.User, svc.Pass = "sa", "SQLpass-123"
		if got, _ := MssqlCrack(svc); got != CrackSuccess {
			t.Errorf("mssql success = %v, want CrackSuccess", got)
		}
	})
	t.Run("fail", func(t *testing.T) {
		svc.User, svc.Pass = "sa", "wrongpass"
		if got, _ := MssqlCrack(svc); got != CrackFail {
			t.Errorf("mssql fail = %v, want CrackFail", got)
		}
	})
}

// --- MongoDB --------------------------------------------------------------

func TestDockerMongodbCrack(t *testing.T) {
	dockerEnabled(t)
	const port = 27017
	// mongo:3.6 兼容 mgo 旧协议。开启 auth 需要 keyfile/root 用户, 较繁琐;
	// 这里用无 auth 模式, 验证未授权探测路径(unauth 命中)。
	startContainer(t, containerSpec{
		image: "mongo:3.6", name: "cm-mongo",
		hostPort: port, guestPort: 27017,
	}, tcpReady(port))
	time.Sleep(5 * time.Second)

	svc := &Service{Ip: "127.0.0.1", Port: port, Protocol: "mongodb", Timeout: 8}
	// 未授权模式下, MongodbUnAuth 探测应命中
	if got, _ := MongodbCrack(svc); got != CrackSuccess {
		t.Errorf("mongodb unauth = %v, want CrackSuccess", got)
	}
}

// --- Oracle ---------------------------------------------------------------

func TestDockerOracleCrack(t *testing.T) {
	dockerEnabled(t)
	const port = 11521
	// gvenzl/oracle-xe: 默认 SYSTEM/OracleXE 密码可由 APP_USER_PASSWORD 或 ORACLE_PASSWORD 配置
	startContainer(t, containerSpec{
		image: "gvenzl/oracle-xe:11-slim", name: "cm-oracle",
		hostPort: port, guestPort: 1521,
		env: []string{
			"ORACLE_PASSWORD=oraclepass",
			"APP_USER=oracle",
			"APP_USER_PASSWORD=oraclepass",
		},
	}, tcpReady(port))
	// oracle-xe 初始化慢(建库), 多等
	time.Sleep(45 * time.Second)

	svc := &Service{Ip: "127.0.0.1", Port: port, Protocol: "oracle", Timeout: 12}
	// 插件会尝试 service: orcl, xe, oracle; gvenzl 镜像用 XE
	// 凭据: system 用户的密码 = ORACLE_PASSWORD
	t.Run("success", func(t *testing.T) {
		svc.User, svc.Pass = "system", "oraclepass"
		if got, _ := OracleCrack(svc); got != CrackSuccess {
			t.Errorf("oracle success = %v, want CrackSuccess (service names tried: orcl/xe/oracle)", got)
		}
	})
	t.Run("fail", func(t *testing.T) {
		svc.User, svc.Pass = "system", "wrongpass"
		if got, _ := OracleCrack(svc); got != CrackFail {
			t.Errorf("oracle fail = %v, want CrackFail", got)
		}
	})
}

// --- SMB ------------------------------------------------------------------

func TestDockerSmbCrack(t *testing.T) {
	dockerEnabled(t)
	const port = 10445
	// dperson/samba: 设置 SMB 用户与共享
	startContainer(t, containerSpec{
		image: "dperson/samba:latest", name: "cm-smb",
		hostPort: port, guestPort: 445,
		cmd: []string{
			"-u", "smbuser;smbpass",
			"-s", "share;/tmp;",
		},
	}, tcpReady(port))
	time.Sleep(8 * time.Second)

	svc := &Service{Ip: "127.0.0.1", Port: port, Protocol: "smb", Timeout: 10}
	t.Run("success", func(t *testing.T) {
		svc.User, svc.Pass = "smbuser", "smbpass"
		if got, _ := SmbCrack(svc); got != CrackSuccess {
			t.Errorf("smb success = %v, want CrackSuccess", got)
		}
	})
	t.Run("fail", func(t *testing.T) {
		svc.User, svc.Pass = "smbuser", "wrongpass"
		if got, _ := SmbCrack(svc); got != CrackFail {
			t.Errorf("smb fail = %v, want CrackFail", got)
		}
	})
}

// --- 不可达目标的 error 路径(无需容器) ---------------------------------------

// TestDockerErrorPath 固化各协议对"不可达目标"的实际返回码, 作为行为回归基线。
//
// 说明: 上游插件只对错误信息含 "timeout" 的才判 CrackError, 对 "connection refused"
// (端口被 RST) 这类立刻失败会被部分插件归为 CrackFail。这是既有设计, 本测试如实记录,
// 不强行纠正。端口 1 在本机几乎必然 connection refused。
func TestDockerErrorPath(t *testing.T) {
	dockerEnabled(t)
	// 每个协议对"不可达(连接被拒)"的预期返回码(经实测确定)
	tests := []struct {
		proto string
		want  int
	}{
		{"mysql", CrackError},     // 错误码非 1045 即 CrackError
		{"memcached", CrackError}, // Dial 失败直接 CrackError
		{"ftp", CrackError},       // Dial 失败直接 CrackError
		{"postgres", CrackFail},   // pq 把连接拒绝当作普通错误 → CrackFail
		{"mssql", CrackFail},      // go-mssqldb 连接拒绝不含 "timeout" → CrackFail
		{"redis", CrackFail},      // go-redis 连接拒绝不含 "timeout" → CrackFail
	}
	for _, tc := range tests {
		t.Run(tc.proto, func(t *testing.T) {
			fn := ScanFuncMap[tc.proto]
			if fn == nil {
				t.Fatalf("no ScanFunc for %s", tc.proto)
			}
			svc := &Service{Ip: "127.0.0.1", Port: 1, Protocol: tc.proto, User: "u", Pass: "p", Timeout: 1}
			got, _ := fn(svc)
			if got != tc.want {
				t.Errorf("%s on unreachable (conn refused) = %v, want %v",
					tc.proto, got, resultCodeName(tc.want))
			}
		})
	}
}

// resultCodeName 返回返回码的可读名, 便于失败信息。
func resultCodeName(c int) string {
	switch c {
	case CrackError:
		return "CrackError"
	case CrackFail:
		return "CrackFail"
	case CrackSuccess:
		return "CrackSuccess"
	}
	return fmt.Sprintf("code(%d)", c)
}
