package utils

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// ExpandIPs 把 IP 规范展开为单个 IP 列表。支持三种格式:
//   - 单 IP:           "192.168.1.1"
//   - CIDR:            "192.168.1.0/24"  (最多 /24, 超出报错防误爆)
//   - IP 段(末位范围): "192.168.1.1-128" (展开 192.168.1.1 ~ 192.168.1.128)
//   - 逗号分隔列表:    "192.168.1.1,192.168.1.2"
//
// 输入会 TrimSpace。非法格式返回错误。
func ExpandIPs(spec string) ([]string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("empty ip spec")
	}

	// 逗号分隔列表: 递归展开每段
	if strings.Contains(spec, ",") {
		var all []string
		for _, part := range strings.Split(spec, ",") {
			ips, err := ExpandIPs(part)
			if err != nil {
				return nil, err
			}
			all = append(all, ips...)
		}
		return all, nil
	}

	// CIDR
	if strings.Contains(spec, "/") {
		return expandCIDR(spec)
	}

	// IP 段 (a.b.c.start-end)
	if strings.Contains(spec, "-") {
		return expandRange(spec)
	}

	// 单 IP: 校验合法性
	if net.ParseIP(spec) == nil {
		return nil, fmt.Errorf("invalid ip: %q", spec)
	}
	return []string{spec}, nil
}

// expandCIDR 展开 CIDR, 限制掩码 >= 24(即最多 256 个地址), 防止 0.0.0.0/0 这类误爆。
func expandCIDR(spec string) ([]string, error) {
	_, ipnet, err := net.ParseCIDR(spec)
	if err != nil {
		return nil, fmt.Errorf("invalid cidr %q: %v", spec, err)
	}

	ones, bits := ipnet.Mask.Size()
	if bits == 0 {
		return nil, fmt.Errorf("invalid cidr mask: %q", spec)
	}
	// 限制: IPv4 掩码 >= 24 (最多 256 地址), IPv6 >= 120
	minOnes := 24
	if bits == 128 {
		minOnes = 120
	}
	if ones < minOnes {
		return nil, fmt.Errorf("cidr %q too large (mask /%d < /%d), max %d addresses allowed to prevent accidental mass scan",
			spec, ones, minOnes, 1<<(bits-minOnes))
	}

	var ips []string
	for ip := ipnet.IP.Mask(ipnet.Mask); ipnet.Contains(ip); incIP(ip) {
		// 复制一份(ip 会被 incIP 原地修改)
		dup := make(net.IP, len(ip))
		copy(dup, ip)
		ips = append(ips, dup.String())
	}
	return ips, nil
}

// expandRange 展开 "a.b.c.start-end" 格式。
func expandRange(spec string) ([]string, error) {
	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid ip range: %q", spec)
	}
	base := parts[0]
	lastDot := strings.LastIndex(base, ".")
	if lastDot < 0 {
		return nil, fmt.Errorf("invalid ip range (need a.b.c.n-m): %q", spec)
	}
	prefix := base[:lastDot] // a.b.c
	startStr := base[lastDot+1:]
	endStr := parts[1]

	start, err := strconv.Atoi(startStr)
	if err != nil || start < 0 || start > 255 {
		return nil, fmt.Errorf("invalid range start %q in %q", startStr, spec)
	}
	end, err := strconv.Atoi(endStr)
	if err != nil || end < 0 || end > 255 {
		return nil, fmt.Errorf("invalid range end %q in %q", endStr, spec)
	}
	if end < start {
		return nil, fmt.Errorf("range end %d < start %d in %q", end, start, spec)
	}

	var ips []string
	for i := start; i <= end; i++ {
		ipStr := fmt.Sprintf("%s.%d", prefix, i)
		if net.ParseIP(ipStr) == nil {
			return nil, fmt.Errorf("invalid ip in range: %q", ipStr)
		}
		ips = append(ips, ipStr)
	}
	return ips, nil
}

// incIP 把 IP 原地 +1。
func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
