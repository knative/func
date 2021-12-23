package ssh

import (
	"net"
	"strings"

	"gopkg.in/natefinch/npipe.v2"
)

func dialSSHAgentConnection(sock string) (agentConn net.Conn, error error) {
	if strings.Contains(sock, "\\pipe\\") {
		agentConn, error = npipe.Dial(sock)
	} else {
		agentConn, error = net.Dial("unix", sock)
	}
	return
}
