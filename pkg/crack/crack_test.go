package crack

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"crack-modify/pkg/crack/plugins"
)

// TestParseTargets 覆盖 ip:port 解析(含 CIDR/段/逗号)及非法输入跳过。
// 注意: 协议不再由 ParseTargets 决定(已废弃 |协议 与端口识别),
// 所以 Protocol 字段应为空, 由调用方按 -m 填充。端口任意。
func TestParseTargets(t *testing.T) {
	tests := []struct {
		name    string
		targets []string
		want    []*IpAddr
	}{
		{
			name:    "single_ip_port",
			targets: []string{"1.2.3.4:3306"},
			want:    []*IpAddr{{Ip: "1.2.3.4", Port: 3306}},
		},
		{
			name:    "nondefault_port_accepted", // 端口任意, 不再识别协议
			targets: []string{"1.2.3.4:3307"},
			want:    []*IpAddr{{Ip: "1.2.3.4", Port: 3307}},
		},
		{
			name:    "arbitrary_high_port",
			targets: []string{"10.0.0.1:65535"},
			want:    []*IpAddr{{Ip: "10.0.0.1", Port: 65535}},
		},
		{
			name: "mixed_ports",
			targets: []string{
				"1.2.3.4:3306",
				"1.2.3.4:3307",
				"5.6.7.8:22",
			},
			want: []*IpAddr{
				{Ip: "1.2.3.4", Port: 3306},
				{Ip: "1.2.3.4", Port: 3307},
				{Ip: "5.6.7.8", Port: 22},
			},
		},
		{
			name:    "trim_whitespace",
			targets: []string{"  1.2.3.4:3306  "},
			want:    []*IpAddr{{Ip: "1.2.3.4", Port: 3306}},
		},
		// 非法/应跳过的输入
		{
			name:    "no_colon_skipped",
			targets: []string{"noport"},
			want:    nil,
		},
		{
			name:    "too_many_colons_skipped",
			targets: []string{"a:b:c"},
			want:    nil,
		},
		{
			name:    "empty_string_skipped",
			targets: []string{""},
			want:    nil,
		},
		{
			name:    "non_numeric_port_skipped", // 端口非数字被跳过
			targets: []string{"1.2.3.4:abc"},
			want:    nil,
		},
		{
			name:    "proto_suffix_now_skipped", // |协议 已废弃, 含 | 的端口解析失败被跳过
			targets: []string{"1.2.3.4:3306|mysql"},
			want:    nil,
		},
		{
			name: "mixed_valid_and_invalid",
			targets: []string{
				"garbage",
				"1.2.3.4:3306",     // valid
				"1.2.3.4:abc",      // 非数字端口
				"1.2.3.4:22|ssh",   // 旧 |协议 语法, 现被跳过
				"5.5.5.5:6379",     // valid
			},
			want: []*IpAddr{
				{Ip: "1.2.3.4", Port: 3306},
				{Ip: "5.5.5.5", Port: 6379},
			},
		},
		{
			name:    "cidr_expand",
			targets: []string{"192.168.1.0/30:3306"},
			want: []*IpAddr{
				{Ip: "192.168.1.0", Port: 3306},
				{Ip: "192.168.1.1", Port: 3306},
				{Ip: "192.168.1.2", Port: 3306},
				{Ip: "192.168.1.3", Port: 3306},
			},
		},
		{
			name:    "range_expand",
			targets: []string{"10.0.0.10-12:6379"},
			want: []*IpAddr{
				{Ip: "10.0.0.10", Port: 6379},
				{Ip: "10.0.0.11", Port: 6379},
				{Ip: "10.0.0.12", Port: 6379},
			},
		},
		{
			name:    "cidr_nondefault_port", // CIDR + 非默认端口(协议由 -m 定, 端口任意)
			targets: []string{"172.16.0.0/31:445"},
			want: []*IpAddr{
				{Ip: "172.16.0.0", Port: 445},
				{Ip: "172.16.0.1", Port: 445},
			},
		},
		{
			name: "comma_list_expand",
			targets: []string{"10.0.0.1,10.0.0.2:22"},
			want: []*IpAddr{
				{Ip: "10.0.0.1", Port: 22},
				{Ip: "10.0.0.2", Port: 22},
			},
		},
		{
			name:    "cidr_too_large_skipped",
			targets: []string{"10.0.0.0/8:3306", "192.168.1.1:3306"},
			want: []*IpAddr{
				{Ip: "192.168.1.1", Port: 3306},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseTargets(tc.targets)
			if !equalIpAddrs(got, tc.want) {
				t.Errorf("ParseTargets(%v) = %+v, want %+v", tc.targets, got, tc.want)
			}
		})
	}
}

func equalIpAddrs(a, b []*IpAddr) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !reflect.DeepEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// TestNewRunnerInjectsDefaults 验证 v0.1 关键改动:
// 当 Options 的 UserMap/CommonPass/TemplatePass 为空时,
// NewRunner 应注入 pkg 内置默认字典。
func TestNewRunnerInjectsDefaults(t *testing.T) {
	r, err := NewRunner(&Options{
		Threads: 4,
		Timeout: 5,
	})
	if err != nil {
		t.Fatalf("NewRunner() error: %v", err)
	}
	if r.options.UserMap == nil || len(r.options.UserMap) == 0 {
		t.Error("NewRunner did not inject default UserMap")
	}
	if !reflect.DeepEqual(r.options.UserMap, userMap) {
		t.Error("injected UserMap != builtin userMap")
	}
	if len(r.options.CommonPass) == 0 || !reflect.DeepEqual(r.options.CommonPass, commonPass) {
		t.Error("NewRunner did not inject default CommonPass correctly")
	}
	if len(r.options.TemplatePass) == 0 || !reflect.DeepEqual(r.options.TemplatePass, templatePass) {
		t.Error("NewRunner did not inject default TemplatePass correctly")
	}
	// 关键: mysql 默认用户名 root, redis/memcached 为空串
	if users, ok := r.options.UserMap["mysql"]; !ok || len(users) == 0 || users[0] != "root" {
		t.Errorf("default mysql users = %v, want [root]", users)
	}
}

// TestNewRunnerKeepsExplicit 验证显式提供的字典不被覆盖。
func TestNewRunnerKeepsExplicit(t *testing.T) {
	customUsers := map[string][]string{"mysql": {"customuser"}}
	customPass := []string{"custompass"}
	tmplPass := []string{"{user}custom"}
	r, err := NewRunner(&Options{
		Threads:      1,
		Timeout:      1,
		UserMap:      customUsers,
		CommonPass:   customPass,
		TemplatePass: tmplPass,
	})
	if err != nil {
		t.Fatalf("NewRunner() error: %v", err)
	}
	if !reflect.DeepEqual(r.options.UserMap, customUsers) {
		t.Error("NewRunner overwrote explicit UserMap")
	}
	if !reflect.DeepEqual(r.options.CommonPass, customPass) {
		t.Error("NewRunner overwrote explicit CommonPass")
	}
	if !reflect.DeepEqual(r.options.TemplatePass, tmplPass) {
		t.Error("NewRunner overwrote explicit TemplatePass")
	}
}

// TestNewRunnerClampsInvalidOptions 验证 B1 修复:
// Threads<1(会死锁)/Threads 过大/Timeout<1(未定义)/Delay<0 都应被兜底到合法范围。
func TestNewRunnerClampsInvalidOptions(t *testing.T) {
	r, err := NewRunner(&Options{
		Threads: 0,    // 非法: 会造成 channel 容量 0 + 无 worker 死锁
		Timeout: 0,    // 非法: net.DialTimeout(...,0) 行为未定义
		Delay:   -5,   // 非法: 负值无意义
	})
	if err != nil {
		t.Fatalf("NewRunner() error: %v", err)
	}
	if r.options.Threads < 1 {
		t.Errorf("Threads = %d, want >= 1 (clamp)", r.options.Threads)
	}
	if r.options.Timeout < 1 {
		t.Errorf("Timeout = %d, want >= 1 (clamp)", r.options.Timeout)
	}
	if r.options.Delay < 0 {
		t.Errorf("Delay = %d, want >= 0 (clamp)", r.options.Delay)
	}

	// 关键: Threads<1 兜底后 Run 不应死锁, 应正常完成
	// 用未导出的 mock 不可行(本文件是 package crack 但 mock helper 在 engine_test),
	// 这里直接用 ScanFuncMap 注入一个 fake(同 withMockPlugin 思路) + 死端口。
	orig := plugins.ScanFuncMap["mysql"]
	plugins.ScanFuncMap["mysql"] = func(s *plugins.Service) (int, error) {
		return plugins.CrackError, fmt.Errorf("test")
	}
	defer func() { plugins.ScanFuncMap["mysql"] = orig }()

	done := make(chan struct{})
	go func() {
		addrs := []*IpAddr{{Ip: "127.0.0.1", Port: 3306, Protocol: "mysql"}}
		r.Run(addrs, []string{"root"}, []string{"p1", "p2"})
		close(done)
	}()
	select {
	case <-done:
		// 正常完成, 未死锁
	case <-time.After(10 * time.Second):
		t.Fatal("Run with clamped Threads<1 deadlocked (B1 regression)")
	}
}

// TestSupportedProtocols 验证支持的协议列表完整(13 个)且每个都被 IsSupportedProtocol 认可。
// 已废弃端口识别, 协议完全由 -m 决定。
func TestSupportedProtocols(t *testing.T) {
	wantProtocols := []string{
		"ftp", "ssh", "wmi", "wmihash", "smb", "mssql",
		"oracle", "mysql", "rdp", "postgres", "redis", "memcached", "mongodb",
	}
	if len(SupportedProtocols) != 13 {
		t.Errorf("SupportedProtocols has %d entries, want 13", len(SupportedProtocols))
	}
	for _, p := range wantProtocols {
		if !IsSupportedProtocol(p) {
			t.Errorf("IsSupportedProtocol(%q) = false, want true", p)
		}
	}
	// 非法协议应返回 false
	if IsSupportedProtocol("all") {
		t.Error("IsSupportedProtocol(\"all\") should be false (all 已废弃)")
	}
	if IsSupportedProtocol("notaproto") {
		t.Error("IsSupportedProtocol(\"notaproto\") should be false")
	}
}

// TestDefaultDictionariesSanity 验证内置字典的非空与基本形态。
func TestDefaultDictionariesSanity(t *testing.T) {
	if len(userMap) == 0 {
		t.Fatal("builtin userMap is empty")
	}
	if len(commonPass) < 50 {
		t.Errorf("builtin commonPass too short: %d", len(commonPass))
	}
	if len(templatePass) < 10 {
		t.Errorf("builtin templatePass too short: %d", len(templatePass))
	}
	// templatePass 的每条都应包含 {user} 占位符
	for i, p := range templatePass {
		if !contains(p, "{user}") {
			t.Errorf("templatePass[%d]=%q does not contain {user}", i, p)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
