package crack

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"crack-modify/pkg/crack/plugins"
)

// TestParseTargets 覆盖两种目标格式以及各种非法输入的跳过逻辑。
func TestParseTargets(t *testing.T) {
	tests := []struct {
		name    string
		targets []string
		want    []*IpAddr
	}{
		{
			name:    "by_port_mysql",
			targets: []string{"1.2.3.4:3306"},
			want:    []*IpAddr{{Ip: "1.2.3.4", Port: 3306, Protocol: "mysql"}},
		},
		{
			name:    "by_port_redis",
			targets: []string{"10.0.0.1:6379"},
			want:    []*IpAddr{{Ip: "10.0.0.1", Port: 6379, Protocol: "redis"}},
		},
		{
			name:    "explicit_protocol_nondefault_port",
			targets: []string{"1.2.3.4:3307|mysql"},
			want:    []*IpAddr{{Ip: "1.2.3.4", Port: 3307, Protocol: "mysql"}},
		},
		{
			name:    "explicit_wmihash",
			targets: []string{"1.2.3.4:135|wmihash"},
			want:    []*IpAddr{{Ip: "1.2.3.4", Port: 135, Protocol: "wmihash"}},
		},
		{
			name: "mixed_valid",
			targets: []string{
				"1.2.3.4:3306",
				"1.2.3.4:3307|mysql",
				"5.6.7.8:22",
			},
			want: []*IpAddr{
				{Ip: "1.2.3.4", Port: 3306, Protocol: "mysql"},
				{Ip: "1.2.3.4", Port: 3307, Protocol: "mysql"},
				{Ip: "5.6.7.8", Port: 22, Protocol: "ssh"},
			},
		},
		{
			name:    "trim_whitespace",
			targets: []string{"  1.2.3.4:3306  "},
			want:    []*IpAddr{{Ip: "1.2.3.4", Port: 3306, Protocol: "mysql"}},
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
			name:    "unknown_port_skipped",
			targets: []string{"1.2.3.4:1234"},
			want:    nil,
		},
		{
			name:    "unknown_protocol_skipped",
			targets: []string{"1.2.3.4:3306|notaproto"},
			want:    nil,
		},
		{
			name: "mixed_valid_and_invalid",
			targets: []string{
				"garbage",
				"1.2.3.4:3306",      // valid
				"1.2.3.4:1234",      // unknown port
				"1.2.3.4:22|bogus",  // unknown proto
				"5.5.5.5:6379",      // valid
			},
			want: []*IpAddr{
				{Ip: "1.2.3.4", Port: 3306, Protocol: "mysql"},
				{Ip: "5.5.5.5", Port: 6379, Protocol: "redis"},
			},
		},
		{
			name:    "cidr_expand",
			targets: []string{"192.168.1.0/30:3306"},
			want: []*IpAddr{
				{Ip: "192.168.1.0", Port: 3306, Protocol: "mysql"},
				{Ip: "192.168.1.1", Port: 3306, Protocol: "mysql"},
				{Ip: "192.168.1.2", Port: 3306, Protocol: "mysql"},
				{Ip: "192.168.1.3", Port: 3306, Protocol: "mysql"},
			},
		},
		{
			name:    "range_expand",
			targets: []string{"10.0.0.10-12:6379"},
			want: []*IpAddr{
				{Ip: "10.0.0.10", Port: 6379, Protocol: "redis"},
				{Ip: "10.0.0.11", Port: 6379, Protocol: "redis"},
				{Ip: "10.0.0.12", Port: 6379, Protocol: "redis"},
			},
		},
		{
			name:    "cidr_with_explicit_protocol",
			targets: []string{"172.16.0.0/31:445|smb"},
			want: []*IpAddr{
				{Ip: "172.16.0.0", Port: 445, Protocol: "smb"},
				{Ip: "172.16.0.1", Port: 445, Protocol: "smb"},
			},
		},
		{
			name: "comma_list_expand",
			targets: []string{"10.0.0.1,10.0.0.2:22"},
			want: []*IpAddr{
				{Ip: "10.0.0.1", Port: 22, Protocol: "ssh"},
				{Ip: "10.0.0.2", Port: 22, Protocol: "ssh"},
			},
		},
		{
			name:    "cidr_too_large_skipped",
			targets: []string{"10.0.0.0/8:3306", "192.168.1.1:3306"},
			want: []*IpAddr{
				{Ip: "192.168.1.1", Port: 3306, Protocol: "mysql"},
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

// TestFilterModule 验证模块过滤。
func TestFilterModule(t *testing.T) {
	addrs := []*IpAddr{
		{Ip: "1.1.1.1", Port: 3306, Protocol: "mysql"},
		{Ip: "1.1.1.2", Port: 22, Protocol: "ssh"},
		{Ip: "1.1.1.3", Port: 6379, Protocol: "redis"},
		{Ip: "1.1.1.4", Port: 3307, Protocol: "mysql"},
	}

	// "all" 返回全部
	if got := FilterModule(addrs, "all"); len(got) != len(addrs) {
		t.Errorf("FilterModule(all) len = %d, want %d", len(got), len(addrs))
	}

	// 指定 mysql 只返回 mysql
	got := FilterModule(addrs, "mysql")
	if len(got) != 2 {
		t.Fatalf("FilterModule(mysql) len = %d, want 2", len(got))
	}
	for _, a := range got {
		if a.Protocol != "mysql" {
			t.Errorf("FilterModule(mysql) returned non-mysql: %+v", a)
		}
	}

	// 不存在的模块返回空
	if got := FilterModule(addrs, "rdp"); len(got) != 0 {
		t.Errorf("FilterModule(rdp) len = %d, want 0", len(got))
	}

	// 空输入返回空
	if got := FilterModule(nil, "all"); len(got) != 0 {
		t.Errorf("FilterModule(nil, all) len = %d, want 0", len(got))
	}
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

// TestProtocolRegistryConsistency 验证 PortNames 与 SupportProtocols 的一致性,
// 以及 13 个协议全部受支持。
func TestProtocolRegistryConsistency(t *testing.T) {
	// 所有 PortNames 指向的协议必须在 SupportProtocols 中
	for port, proto := range PortNames {
		if !SupportProtocols[proto] {
			t.Errorf("PortNames[%d]=%q is not in SupportProtocols", port, proto)
		}
	}

	wantProtocols := []string{
		"ftp", "ssh", "wmi", "wmihash", "smb", "mssql",
		"oracle", "mysql", "rdp", "postgres", "redis", "memcached", "mongodb",
	}
	for _, p := range wantProtocols {
		if !SupportProtocols[p] {
			t.Errorf("SupportProtocols missing %q", p)
		}
	}
	if len(SupportProtocols) != 13 {
		t.Errorf("SupportProtocols has %d entries, want 13", len(SupportProtocols))
	}

	// 关键端口映射正确
	keyPorts := map[int]string{
		21: "ftp", 22: "ssh", 135: "wmi", 445: "smb", 1433: "mssql",
		1521: "oracle", 3306: "mysql", 3389: "rdp", 5432: "postgres",
		6379: "redis", 11211: "memcached", 27017: "mongodb",
	}
	for port, want := range keyPorts {
		if got := PortNames[port]; got != want {
			t.Errorf("PortNames[%d] = %q, want %q", port, got, want)
		}
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
