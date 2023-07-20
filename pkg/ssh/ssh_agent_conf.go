//go:build !windows
// +build !windows

package ssh

import (
	"net"
)

func dialSSHAgentConnection(sock string) (agentConn net.Conn, error error) {
	return net.Dial("unix", sock)
}
