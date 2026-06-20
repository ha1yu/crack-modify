package plugins

import (
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// startTCPServer 拉起一个临时 TCP 服务器: 收到任意数据后按 handler 返回响应。
// 返回端口号; 测试结束自动关闭。handler 为 nil 时只接受连接不回数据。
func startTCPServer(t *testing.T, handler func(conn net.Conn, received []byte)) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	var wg sync.WaitGroup
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				defer c.Close()
				_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
				buf := make([]byte, 1024)
				n, _ := c.Read(buf)
				if handler != nil {
					handler(c, buf[:n])
				}
			}(c)
		}
	}()
	// 让 Close 等待在途连接处理完, 避免端口复用干扰后续用例
	t.Cleanup(func() { wg.Wait() })
	return l.Addr().(*net.TCPAddr).Port
}

// --- Memcached ---------------------------------------------------------------

// TestMemcachedCrackUnauthSuccess: mock 回含 STAT 的响应 → CrackSuccess。
func TestMemcachedCrackUnauthSuccess(t *testing.T) {
	port := startTCPServer(t, func(c net.Conn, received []byte) {
		// memcached stats 响应
		_, _ = c.Write([]byte("STAT pid 1234\r\nSTAT uptime 100\r\nEND\r\n"))
	})
	svc := &Service{Ip: "127.0.0.1", Port: port, Protocol: "memcached", Timeout: 2}
	got, err := MemcachedCrack(svc)
	if got != CrackSuccess {
		t.Errorf("MemcachedCrack(unauth mock) = %v (err=%v), want CrackSuccess", got, err)
	}
}

// TestMemcachedCrackNoStat: mock 回不含 STAT → 非 success。
func TestMemcachedCrackNoStat(t *testing.T) {
	port := startTCPServer(t, func(c net.Conn, received []byte) {
		_, _ = c.Write([]byte("NOT_A_MEMCACHED_SERVER\r\n"))
	})
	svc := &Service{Ip: "127.0.0.1", Port: port, Protocol: "memcached", Timeout: 2}
	got, _ := MemcachedCrack(svc)
	if got == CrackSuccess {
		t.Error("MemcachedCrack(non-STAT mock) should not be CrackSuccess")
	}
}

// TestMemcachedCrackConnError: 连不上 → CrackError。
func TestMemcachedCrackConnError(t *testing.T) {
	// 选一个确定未被监听的端口
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := l.Addr().(*net.TCPAddr).Port
	l.Close()

	svc := &Service{Ip: "127.0.0.1", Port: dead, Protocol: "memcached", Timeout: 1}
	got, err := MemcachedCrack(svc)
	if got != CrackError {
		t.Errorf("MemcachedCrack(unreachable) = %v (err=%v), want CrackError", got, err)
	}
}

// TestMemcachedCrackSendsStatsCommand: 验证插件确实发送了 stats 命令。
func TestMemcachedCrackSendsStatsCommand(t *testing.T) {
	var received string
	port := startTCPServer(t, func(c net.Conn, recv []byte) {
		received = string(recv)
		_, _ = c.Write([]byte("STAT ok\r\nEND\r\n"))
	})
	svc := &Service{Ip: "127.0.0.1", Port: port, Protocol: "memcached", Timeout: 2}
	MemcachedCrack(svc)
	if !strings.Contains(received, "stats") {
		t.Errorf("memcached plugin did not send 'stats' command; received=%q", received)
	}
}

// --- MongoDB (unauth path) ---------------------------------------------------

// TestMongodbUnAuthSuccess: mock 回含 totalLinesWritten → CrackSuccess。
func TestMongodbUnAuthSuccess(t *testing.T) {
	port := startTCPServer(t, func(c net.Conn, received []byte) {
		// 只要响应里包含 totalLinesWritten 即触发未授权命中
		_, _ = c.Write([]byte("some-header totalLinesWritten=42 trailing"))
	})
	svc := &Service{Ip: "127.0.0.1", Port: port, Protocol: "mongodb", Timeout: 2}
	got, err := MongodbUnAuth(svc)
	if got != CrackSuccess {
		t.Errorf("MongodbUnAuth(unauth mock) = %v (err=%v), want CrackSuccess", got, err)
	}
}

// TestMongodbUnAuthFallback: mock 回不含 totalLinesWritten → 返回 -1(交由后续认证流程)。
func TestMongodbUnAuthFallback(t *testing.T) {
	port := startTCPServer(t, func(c net.Conn, received []byte) {
		_, _ = c.Write([]byte("not-mongo-response"))
	})
	svc := &Service{Ip: "127.0.0.1", Port: port, Protocol: "mongodb", Timeout: 2}
	got, _ := MongodbUnAuth(svc)
	if got != -1 {
		t.Errorf("MongodbUnAuth(non-mongo mock) = %v, want -1 (fallback)", got)
	}
}

// TestMongodbUnAuthConnError: 连不上 → CrackError。
func TestMongodbUnAuthConnError(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := l.Addr().(*net.TCPAddr).Port
	l.Close()

	svc := &Service{Ip: "127.0.0.1", Port: dead, Protocol: "mongodb", Timeout: 1}
	got, err := MongodbUnAuth(svc)
	if got != CrackError {
		t.Errorf("MongodbUnAuth(unreachable) = %v (err=%v), want CrackError", got, err)
	}
}
