package crack

import (
	"strconv"
	"strings"

	"crack-modify/internal/utils"
)

// ParseTargets 解析目标列表。每行格式为 "ip:port"。
//
// 协议不再在此处确定(已废弃 |协议 语法与端口识别):
//   - ip 部分支持 单个IP / CIDR(≤/24) / IP段 / 逗号列表, 由 utils.ExpandIPs 展开
//   - port 必须是纯数字
//   - 每个目标的 Protocol 字段留空, 由调用方根据 -m 指定的协议统一填充
//
// 非法行(无端口、端口非数字、端口含 | 等)被跳过。
func ParseTargets(targets []string) (results []*IpAddr) {
	for _, target := range targets {
		target = strings.TrimSpace(target)
		tmp := strings.Split(target, ":")
		if len(tmp) != 2 {
			continue
		}
		// 展开目标 IP: 支持单 IP / CIDR / IP段 / 逗号列表
		ips, err := utils.ExpandIPs(tmp[0])
		if err != nil {
			continue
		}
		// 端口必须是纯数字(不再支持 |协议)
		port, err := strconv.Atoi(tmp[1])
		if err != nil {
			continue
		}
		for _, ip := range ips {
			results = append(results, &IpAddr{
				Ip:   ip,
				Port: port,
				// Protocol 留空, 由调用方按 -m 填充
			})
		}
	}

	return
}
