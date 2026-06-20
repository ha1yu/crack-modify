package plugins

import (
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

func SshCrack(serv *Service) (int, error) {
	config := &ssh.ClientConfig{
		User: serv.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(serv.Pass),
		},
		Timeout: time.Duration(serv.Timeout) * time.Second,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
	}
	client, err := ssh.Dial("tcp", fmt.Sprintf("%v:%v", serv.Ip, serv.Port), config)
	if err != nil {
		if strings.Contains(err.Error(), "timeout") {
			return CrackError, err
		}
		return CrackFail, nil
	}
	defer client.Close()
	session, err := client.NewSession()
	// 仅校验会话能否真正执行命令(排除认证通过但被 ForceCommand 等限制的情况)。
	// 用 true(标准 no-op)而非 echo <标记串>, 避免在目标留下可识别的工具指纹。
	errRet := session.Run("true")
	if err != nil || errRet != nil {
		return CrackFail, nil
	}
	defer session.Close()
	return CrackSuccess, nil
}
