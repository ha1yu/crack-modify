package utils

import (
	"sort"
	"testing"
)

// TestRemoveDuplicate 验证去重并过滤空字符串。
// 注意: redis/memcached 的默认用户名是空串, RemoveDuplicate 会丢弃空串,
// 因此调用方(runner)对这些协议的口令字典需另行处理。这里固化该行为边界。
func TestRemoveDuplicate(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "distinct",
			in:   []string{"a", "b", "c"},
			want: []string{"a", "b", "c"},
		},
		{
			name: "with_dup",
			in:   []string{"root", "root", "admin", "admin", "root"},
			want: []string{"root", "admin"},
		},
		{
			name: "empty_strings_dropped",
			in:   []string{"", "", "root", ""},
			want: []string{"root"},
		},
		{
			name: "only_empty",
			in:   []string{"", ""},
			want: nil,
		},
		{
			name: "nil_input",
			in:   nil,
			want: nil,
		},
		{
			name: "single",
			in:   []string{"only"},
			want: []string{"only"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RemoveDuplicate(tc.in)
			if !equalStrSet(got, tc.want) {
				t.Errorf("RemoveDuplicate(%v) = %v, want set %v", tc.in, got, tc.want)
			}
		})
	}
}

// equalStrSet 顺序无关地比较两个字符串切片(集合相等)。
func equalStrSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sa := append([]string{}, a...)
	sb := append([]string{}, b...)
	sort.Strings(sa)
	sort.Strings(sb)
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}

func TestMd5(t *testing.T) {
	// 已知的 MD5 向量, 防止哈希实现被误改
	tests := []struct {
		in   string
		want string
	}{
		{"", "d41d8cd98f00b204e9800998ecf8427e"},
		{"abc", "900150983cd24fb0d6963f7d28e17f72"},
		{"some-random-input", ""}, // 仅校验长度+格式, 见下
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got := Md5(tc.in)
			if len(got) != 32 {
				t.Fatalf("Md5(%q) length = %d, want 32", tc.in, len(got))
			}
			// "" 与 "abc" 是标准已知值, 严格断言
			if (tc.in == "" || tc.in == "abc") && got != tc.want {
				t.Errorf("Md5(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	// 不同输入应产生不同输出
	if Md5("a") == Md5("b") {
		t.Error("Md5 should produce different output for different input")
	}
}

func TestHasStr(t *testing.T) {
	data := []string{"root", "admin", "sa"}
	if !HasStr(data, "root") {
		t.Error("HasStr should find existing element")
	}
	if HasStr(data, "missing") {
		t.Error("HasStr should not find missing element")
	}
	if HasStr(data, "") {
		t.Error("HasStr should not find empty string when absent")
	}
}

func TestSuffixStr(t *testing.T) {
	suffixes := []string{".txt", ".csv", ".log"}
	// 命中
	if got, ok := SuffixStr(suffixes, "result.txt"); !ok || got != ".txt" {
		t.Errorf("SuffixStr(result.txt) = %q,%v, want .txt,true", got, ok)
	}
	// 未命中
	if _, ok := SuffixStr(suffixes, "result.json"); ok {
		t.Error("SuffixStr should not match .json")
	}
}

func TestHasInt(t *testing.T) {
	data := []int{21, 22, 3306, 6379}
	if !HasInt(data, 3306) {
		t.Error("HasInt should find 3306")
	}
	if HasInt(data, 9999) {
		t.Error("HasInt should not find 9999")
	}
}
