// Package agentbin 内嵌各架构 havline-agent 二进制，供 SSH 安装器按 VPS 架构选择。
// 仓库中的占位文件保证开发机构建可通过；Docker 构建阶段会用真实二进制覆盖它们
// （见 Dockerfile：先交叉编译 agent，再构建主程序）。
package agentbin

import (
	_ "embed"
)

//go:embed havline-agent-linux-amd64
var LinuxAMD64 []byte

//go:embed havline-agent-linux-arm64
var LinuxARM64 []byte

// BinaryForArch 返回对应架构的 agent 二进制；arch 取 uname -m 输出（x86_64/aarch64）。
func BinaryForArch(arch string) ([]byte, bool) {
	switch arch {
	case "x86_64", "amd64":
		return LinuxAMD64, true
	case "aarch64", "arm64":
		return LinuxARM64, true
	}
	return nil, false
}
