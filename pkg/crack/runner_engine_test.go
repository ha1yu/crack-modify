package crack

import (
	"fmt"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"crack-modify/pkg/crack/plugins"
)

// withMockPlugin 临时把 ScanFuncMap[protocol] 替换为 fn, 测试结束后恢复原值。
// 这样可以在不依赖任何真实数据库/服务的前提下驱动 runner 的爆破循环。
func withMockPlugin(t *testing.T, protocol string, fn plugins.ScanFunc) {
	t.Helper()
	orig := plugins.ScanFuncMap[protocol]
	plugins.ScanFuncMap[protocol] = fn
	t.Cleanup(func() { plugins.ScanFuncMap[protocol] = orig })
}

// deadPort 返回一个当前确定未被监听的端口: 通过反复尝试连接来验证它确实被拒绝,
// 避免使用"bind 后立即关闭"的端口(TIME_WAIT/复用会让连接偶尔成功, 造成 flaky)。
func deadPort(t *testing.T) int {
	t.Helper()
	for attempt := 0; attempt < 20; attempt++ {
		// 取一个候选端口(绑定再关闭拿到一个空闲高位端口)
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		l.Close()
		// 立刻验证: 连不上才算"死"
		c, derr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
		if derr != nil {
			// 连接失败 → 端口确实未被监听
			return port
		}
		c.Close()
		// 否则被别的东西占用了, 换一个再试
	}
	t.Fatal("could not find a reliably-dead port after 20 attempts")
	return 0
}

// mockListener 在 127.0.0.1 上拉起一个临时 TCP 监听并立即接受/丢弃连接,
// 返回其端口号。用于 CheckAlive 的存活目标。关闭时清理 goroutine。
func mockListener(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

// TestCheckAlive 验证存活探测: 监听端口存活, 未监听端口被过滤。
func TestCheckAlive(t *testing.T) {
	alivePort := mockListener(t)
	deadP := deadPort(t)

	r, err := NewRunner(&Options{Threads: 4, Timeout: 1})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	addrs := []*IpAddr{
		{Ip: "127.0.0.1", Port: alivePort, Protocol: "mysql"},
		{Ip: "127.0.0.1", Port: deadP, Protocol: "mysql"},
	}
	got := r.CheckAlive(addrs)
	if len(got) != 1 {
		t.Fatalf("CheckAlive returned %d addrs, want 1 (alive=%d, dead=%d)", len(got), alivePort, deadP)
	}
	if got[0].Port != alivePort {
		t.Errorf("alive addr port = %d, want %d", got[0].Port, alivePort)
	}
}

// TestCheckAliveEmpty 验证空输入安全返回。
func TestCheckAliveEmpty(t *testing.T) {
	r, _ := NewRunner(&Options{Threads: 2, Timeout: 1})
	if got := r.CheckAlive(nil); len(got) != 0 {
		t.Errorf("CheckAlive(nil) = %d, want 0", len(got))
	}
}

// TestRunStopOnSuccess 验证 CrackAll=false 时命中即停:
// 提供的口令中只有一个是"正确"的, mock 会在命中它时返回 CrackSuccess。
// 由于 CrackAll=false, runner 命中后应停止该目标, 命中次数应为 1。
func TestRunStopOnSuccess(t *testing.T) {
	const correctPass = "THE_RIGHT_ONE"
	var hits int32
	withMockPlugin(t, "mysql", func(s *plugins.Service) (int, error) {
		if s.Pass == correctPass {
			atomic.AddInt32(&hits, 1)
			return plugins.CrackSuccess, nil
		}
		return plugins.CrackFail, nil
	})

	r, _ := NewRunner(&Options{Threads: 1, Timeout: 1, CrackAll: false})
	addrs := []*IpAddr{{Ip: "127.0.0.1", Port: 3306, Protocol: "mysql"}}
	userDict := []string{"root"}
	passDict := []string{"wrong1", "wrong2", correctPass, "wrong3", correctPass}

	results := r.Run(addrs, userDict, passDict)
	if len(results) != 1 {
		t.Errorf("CrackAll=false results = %d, want 1 (hit)", len(results))
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("CrackAll=false success hits = %d, want exactly 1 (stop-on-success)", hits)
	}
	if len(results) > 0 && results[0].UserPass != "root:"+correctPass {
		t.Errorf("result UserPass = %q, want %q", results[0].UserPass, "root:"+correctPass)
	}
}

// TestRunCrackAllContinues 验证 CrackAll=true 时命中后继续爆破,
// 多个不同"正确"口令都应被记录为结果。
func TestRunCrackAllContinues(t *testing.T) {
	// 注意: runner 会对 (ip,port,proto,user,pass) 做 MD5 去重,
	// 因此要触发多次命中必须用互不相同的 pass。
	hitsPass := map[string]bool{"GOOD1": true, "GOOD2": true, "GOOD3": true}
	var hits int32
	withMockPlugin(t, "mysql", func(s *plugins.Service) (int, error) {
		if hitsPass[s.Pass] {
			atomic.AddInt32(&hits, 1)
			return plugins.CrackSuccess, nil
		}
		return plugins.CrackFail, nil
	})

	r, _ := NewRunner(&Options{Threads: 2, Timeout: 1, CrackAll: true})
	addrs := []*IpAddr{{Ip: "127.0.0.1", Port: 3306, Protocol: "mysql"}}
	userDict := []string{"root"}
	passDict := []string{"bad", "GOOD1", "GOOD2", "GOOD3", "bad2"}

	results := r.Run(addrs, userDict, passDict)
	if len(results) != 3 {
		t.Errorf("CrackAll=true results = %d, want 3", len(results))
	}
	if atomic.LoadInt32(&hits) != 3 {
		t.Errorf("CrackAll=true success hits = %d, want 3", hits)
	}
}

// TestRunStopsOnError 验证 CrackError 导致该目标立即停止后续尝试。
func TestRunStopsOnError(t *testing.T) {
	var attempts int32
	withMockPlugin(t, "mysql", func(s *plugins.Service) (int, error) {
		n := atomic.AddInt32(&attempts, 1)
		// 第一次返回 Error(模拟主机不可达), runner 应停止该目标
		if n == 1 {
			return plugins.CrackError, fmt.Errorf("dial timeout")
		}
		return plugins.CrackFail, nil
	})

	r, _ := NewRunner(&Options{Threads: 1, Timeout: 1, CrackAll: true})
	addrs := []*IpAddr{{Ip: "127.0.0.1", Port: 3306, Protocol: "mysql"}}
	userDict := []string{"root"}
	passDict := []string{"p1", "p2", "p3", "p4", "p5"}

	results := r.Run(addrs, userDict, passDict)
	if len(results) != 0 {
		t.Errorf("results = %d, want 0 on CrackError", len(results))
	}
	// Threads=1, 第一次即 Error → stopMap 标记 → 其余任务应被跳过
	if got := atomic.LoadInt32(&attempts); got > 2 {
		t.Errorf("attempts after CrackError = %d, want <= 2 (target stopped)", got)
	}
}

// TestRunDelayThrottles 验证 Delay>0 时全局限速语义:
// P1 修复后, 限速改用全局 ticker gate, 整体速率 = 1 req / Delay 秒(而非旧的 Threads/Delay)。
// 3 任务 / Delay=1s → 首个令牌在 ~1s 后发出, 共需 ~3s(>= 2s 断言成立)。
func TestRunDelayThrottles(t *testing.T) {
	withMockPlugin(t, "mysql", func(s *plugins.Service) (int, error) {
		return plugins.CrackFail, nil
	})

	r, _ := NewRunner(&Options{Threads: 1, Timeout: 1, Delay: 1, CrackAll: false})
	addrs := []*IpAddr{{Ip: "127.0.0.1", Port: 3306, Protocol: "mysql"}}
	userDict := []string{"root"}
	passDict := []string{"x", "y", "z"} // 3 个任务

	start := time.Now()
	r.Run(addrs, userDict, passDict)
	elapsed := time.Since(start)
	// 全局 gate: 3 任务, 每个等 ~1s 令牌 → 至少 ~2s
	if elapsed < 2*time.Second {
		t.Errorf("with Delay=1s gate and 3 tasks, elapsed = %v, want >= 2s", elapsed)
	}
}

// TestRunDelayGlobalThrottle 验证 P1 的关键语义变化:
// 即便 Threads>1, 全局 gate 仍把整体速率限制为 1 req / Delay 秒,
// 而非旧的 Threads/Delay。用多 worker + 多任务断言总耗时 ≈ tasks*Delay。
func TestRunDelayGlobalThrottle(t *testing.T) {
	withMockPlugin(t, "mysql", func(s *plugins.Service) (int, error) {
		return plugins.CrackFail, nil
	})
	// 4 workers, 6 tasks, Delay=1s → 全局 gate 下总耗时 ~6s(而非旧的 ~1.5s=6/4)
	r, _ := NewRunner(&Options{Threads: 4, Timeout: 1, Delay: 1, CrackAll: false})
	addrs := []*IpAddr{{Ip: "127.0.0.1", Port: 3306, Protocol: "mysql"}}
	userDict := []string{"root", "admin"}
	passDict := []string{"p1", "p2", "p3"} // 6 个任务

	start := time.Now()
	r.Run(addrs, userDict, passDict)
	elapsed := time.Since(start)
	// 6 任务 * 1s/req = ~6s; 旧语义(每 worker sleep)会 ~1.5s。断言 >= 4s 足以区分新旧语义。
	if elapsed < 4*time.Second {
		t.Errorf("global gate: 6 tasks with Delay=1s elapsed = %v, want >= 4s (old per-worker semantics would be ~1.5s)", elapsed)
	}
}

// TestRunTaskDedup 验证 {user} 模板替换 + MD5 任务去重:
// 当 passDict 含重复项时, 同一 (user,pass) 不应被重复执行。
func TestRunTaskDedup(t *testing.T) {
	var calls int32
	withMockPlugin(t, "mysql", func(s *plugins.Service) (int, error) {
		atomic.AddInt32(&calls, 1)
		return plugins.CrackFail, nil
	})
	r, _ := NewRunner(&Options{Threads: 1, Timeout: 1, CrackAll: true})
	addrs := []*IpAddr{{Ip: "127.0.0.1", Port: 3306, Protocol: "mysql"}}
	userDict := []string{"root"}
	// 4 个口令但其中两个完全相同 → 去重后 3 个唯一任务
	passDict := []string{"same", "same", "diff1", "diff2"}

	r.Run(addrs, userDict, passDict)
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("plugin calls = %d, want 3 after dedup", got)
	}
}

// TestRunMultipleAddrs 验证多目标场景下每个目标独立计数与停止。
func TestRunMultipleAddrs(t *testing.T) {
	var hits int32
	withMockPlugin(t, "mysql", func(s *plugins.Service) (int, error) {
		if s.Pass == "hit" {
			atomic.AddInt32(&hits, 1)
			return plugins.CrackSuccess, nil
		}
		return plugins.CrackFail, nil
	})
	r, _ := NewRunner(&Options{Threads: 2, Timeout: 1, CrackAll: false})
	addrs := []*IpAddr{
		{Ip: "127.0.0.1", Port: 3306, Protocol: "mysql"},
		{Ip: "127.0.0.2", Port: 3306, Protocol: "mysql"},
		{Ip: "127.0.0.3", Port: 3306, Protocol: "mysql"},
	}
	userDict := []string{"root"}
	passDict := []string{"miss", "hit", "hit2"}

	results := r.Run(addrs, userDict, passDict)
	// 3 个目标各自命中一次(CrackAll=false), 共 3 条结果
	if len(results) != 3 {
		t.Errorf("results = %d, want 3 (one hit per addr)", len(results))
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("total hits = %d, want 3", got)
	}
}

// BenchmarkCrackLargeDict 观察 P3(去重改 map) 后大字典场景的引擎吞吐。
// mock ScanFunc 极快, 压力集中在任务生成/去重/调度开销上。
func BenchmarkCrackLargeDict(b *testing.B) {
	// 注入极快的 mock(不走网络), 测试后恢复
	orig := plugins.ScanFuncMap["mysql"]
	plugins.ScanFuncMap["mysql"] = func(s *plugins.Service) (int, error) {
		return plugins.CrackFail, nil
	}
	defer func() { plugins.ScanFuncMap["mysql"] = orig }()

	r, _ := NewRunner(&Options{Threads: 8, Timeout: 1, CrackAll: true})
	addrs := []*IpAddr{{Ip: "127.0.0.1", Port: 3306, Protocol: "mysql"}}

	// 构造大字典: 50 用户 × 100 口令 = 5000 任务
	users := make([]string, 50)
	for i := range users {
		users[i] = "user" + strconv.Itoa(i)
	}
	passes := make([]string, 100)
	for i := range passes {
		passes[i] = "pass" + strconv.Itoa(i)
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		r.Run(addrs, users, passes)
	}
}
