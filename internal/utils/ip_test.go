package utils

import (
	"reflect"
	"testing"
)

// TestExpandIPsSingle 验证单 IP 解析。
func TestExpandIPsSingle(t *testing.T) {
	got, err := ExpandIPs("192.168.1.1")
	if err != nil {
		t.Fatalf("ExpandIPs error: %v", err)
	}
	want := []string{"192.168.1.1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestExpandIPsTrimSpace 验证前后空格被 Trim。
func TestExpandIPsTrimSpace(t *testing.T) {
	got, err := ExpandIPs("  10.0.0.5  ")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(got) != 1 || got[0] != "10.0.0.5" {
		t.Errorf("got %v, want [10.0.0.5]", got)
	}
}

// TestExpandIPsCIDR 验证 CIDR 展开(典型 /30 和 /24)。
func TestExpandIPsCIDR(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "/30 four addrs",
			in:   "192.168.1.0/30",
			want: []string{"192.168.1.0", "192.168.1.1", "192.168.1.2", "192.168.1.3"},
		},
		{
			name: "/31 two addrs",
			in:   "10.0.0.0/31",
			want: []string{"10.0.0.0", "10.0.0.1"},
		},
		{
			name: "/32 single",
			in:   "172.16.0.5/32",
			want: []string{"172.16.0.5"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExpandIPs(tc.in)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestExpandIPsCIDRLarge 验证 /24 展开 256 个地址。
func TestExpandIPsCIDRLarge(t *testing.T) {
	got, err := ExpandIPs("192.168.1.0/24")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(got) != 256 {
		t.Errorf("/24 got %d addrs, want 256", len(got))
	}
	if got[0] != "192.168.1.0" || got[255] != "192.168.1.255" {
		t.Errorf("range = %s..%s, want 192.168.1.0..192.168.1.255", got[0], got[255])
	}
}

// TestExpandIPsCIDRTooLarge 验证超过 /24 报错(防误爆)。
func TestExpandIPsCIDRTooLarge(t *testing.T) {
	bad := []string{"10.0.0.0/8", "192.168.0.0/16", "0.0.0.0/0"}
	for _, in := range bad {
		t.Run(in, func(t *testing.T) {
			_, err := ExpandIPs(in)
			if err == nil {
				t.Errorf("expected error for too-large cidr %q", in)
			}
		})
	}
}

// TestExpandIPsRange 验证 IP 段展开。
func TestExpandIPsRange(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"simple", "192.168.1.100-102", []string{"192.168.1.100", "192.168.1.101", "192.168.1.102"}},
		{"single end", "10.0.0.5-5", []string{"10.0.0.5"}},
		{"cross hundred", "172.16.0.98-103", []string{"172.16.0.98", "172.16.0.99", "172.16.0.100", "172.16.0.101", "172.16.0.102", "172.16.0.103"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExpandIPs(tc.in)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestExpandIPsRangeInvalid 验证非法段。
func TestExpandIPsRangeInvalid(t *testing.T) {
	bad := []string{
		"192.168.1.100-50",   // end < start
		"192.168.1.300-301",  // > 255
		"192.168.1.abc-10",   // 非数字
	}
	for _, in := range bad {
		t.Run(in, func(t *testing.T) {
			_, err := ExpandIPs(in)
			if err == nil {
				t.Errorf("expected error for %q", in)
			}
		})
	}
}

// TestExpandIPsCommaList 验证逗号分隔列表。
func TestExpandIPsCommaList(t *testing.T) {
	got, err := ExpandIPs("10.0.0.1,10.0.0.2,10.0.0.3")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	want := []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestExpandIPsCommaMixed 验证逗号列表混用 CIDR/段。
func TestExpandIPsCommaMixed(t *testing.T) {
	got, err := ExpandIPs("10.0.0.1,10.0.0.10-12")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	want := []string{"10.0.0.1", "10.0.0.10", "10.0.0.11", "10.0.0.12"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestExpandIPsInvalid 验证各种非法输入。
func TestExpandIPsInvalid(t *testing.T) {
	bad := []string{
		"",
		"not.an.ip",
		"999.999.999.999",
		"192.168.1.0/33", // 非法掩码
	}
	for _, in := range bad {
		t.Run(in, func(t *testing.T) {
			_, err := ExpandIPs(in)
			if err == nil {
				t.Errorf("expected error for %q", in)
			}
		})
	}
}
