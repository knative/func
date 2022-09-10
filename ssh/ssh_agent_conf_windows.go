package ssh

import (
	"net"
	"strings"

	"github.com/Microsoft/go-winio"
)

func dialSSHAgentConnection(sock string) (agentConn net.Conn, error error) {
	if strings.Contains(sock, "\\pipe\\") {
		agentConn, error = winio.DialPipe(sock, nil)
	} else {
		agentConn, error = net.Dial("unix", sock)
	}
	return
}
