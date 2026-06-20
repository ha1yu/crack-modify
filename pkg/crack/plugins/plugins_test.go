package plugins

import (
	"testing"
)

// TestResultConstants 验证三个返回码的精确值。
// runner 通过 switch resp { case CrackSuccess / CrackError / CrackFail } 分发,
// 值若被改动会破坏爆破循环逻辑, 必须固化。
func TestResultConstants(t *testing.T) {
	if CrackError != 0 {
		t.Errorf("CrackError = %d, want 0", CrackError)
	}
	if CrackFail != 1 {
		t.Errorf("CrackFail = %d, want 1", CrackFail)
	}
	if CrackSuccess != 2 {
		t.Errorf("CrackSuccess = %d, want 2", CrackSuccess)
	}
}

// expectedProtocols 是 init() 中应当注册的全部协议。
var expectedProtocols = []string{
	"ftp", "ssh", "wmi", "wmihash", "smb", "mssql",
	"oracle", "mysql", "rdp", "postgres", "redis", "memcached", "mongodb",
}

// TestScanFuncMapRegistered 验证每个协议都注册了非 nil 的 ScanFunc。
func TestScanFuncMapRegistered(t *testing.T) {
	if ScanFuncMap == nil {
		t.Fatal("ScanFuncMap is nil, init() did not run")
	}
	if len(ScanFuncMap) != len(expectedProtocols) {
		t.Errorf("ScanFuncMap has %d entries, want %d", len(ScanFuncMap), len(expectedProtocols))
	}
	for _, proto := range expectedProtocols {
		fn, ok := ScanFuncMap[proto]
		if !ok {
			t.Errorf("ScanFuncMap missing protocol %q", proto)
			continue
		}
		if fn == nil {
			t.Errorf("ScanFuncMap[%q] is nil", proto)
		}
	}
}

// TestServiceStruct 验证 Service 结构体可正常构造且字段语义正确。
func TestServiceStruct(t *testing.T) {
	s := Service{
		Ip:       "127.0.0.1",
		Port:     3306,
		Protocol: "mysql",
		User:     "root",
		Pass:     "pass",
		Timeout:  5,
	}
	if s.Ip != "127.0.0.1" || s.Port != 3306 || s.Protocol != "mysql" {
		t.Error("Service core fields not set correctly")
	}
	if s.User != "root" || s.Pass != "pass" || s.Timeout != 5 {
		t.Error("Service auth/timeout fields not set correctly")
	}
}

// TestScanFuncMapValuesCallable 验证每个注册的 ScanFunc 都是可安全调用的函数:
// 对一个必然连不上的目标调用, 应返回合法返回码 ∈ {CrackError, CrackFail, CrackSuccess},
// 且不 panic。用短 Timeout 控制耗时。
func TestScanFuncMapValuesCallable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping plugin call smoke test in -short mode")
	}
	// 选几个代表性协议(DB 型、TCP 型、协议型)做调用冒烟, 避免对全部 13 个都做网络调用拖慢测试。
	// 目标使用一个几乎必然拒绝/超时的地址, Timeout=1s 上限。
	protocols := []string{"ftp", "ssh", "mysql", "mssql", "postgres", "redis", "memcached", "mongodb"}
	for _, proto := range protocols {
		t.Run(proto, func(t *testing.T) {
			fn := ScanFuncMap[proto]
			if fn == nil {
				t.Fatalf("no ScanFunc for %s", proto)
			}
			svc := &Service{
				Ip:       "127.0.0.1",
				Port:     1, // 1 号端口通常无服务 → 连接被拒
				Protocol: proto,
				User:     "u",
				Pass:     "p",
				Timeout:  1,
			}
			// 不对具体返回值做断言(各协议对"连不上"的判定 CrackError/CrackFail 不一),
			// 只保证调用不 panic 且返回值合法。
			got, _ := fn(svc)
			if got != CrackError && got != CrackFail && got != CrackSuccess {
				t.Errorf("%s returned invalid code %d", proto, got)
			}
		})
	}
}
