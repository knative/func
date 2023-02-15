package ssh

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// readSecret prompts for a secret and returns value input by user from stdin
// Unlike terminal.ReadPassword(), $(echo $SECRET | podman...) is supported.
// Additionally, all input after `<secret>/n` is queued to podman command.
//
// NOTE: this code is based on "github.com/containers/podman/v3/pkg/terminal"
func readSecret(prompt string) (pw []byte, err error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Fprint(os.Stderr, prompt)
		pw, err = term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		return
	}

	var b [1]byte
	for {
		n, err := os.Stdin.Read(b[:])
		// terminal.readSecret discards any '\r', so we do the same
		if n > 0 && b[0] != '\r' {
			if b[0] == '\n' {
				return pw, nil
			}
			pw = append(pw, b[0])
			// limit size, so that a wrong input won't fill up the memory
			if len(pw) > 1024 {
				err = errors.New("password too long, 1024 byte limit")
			}
		}
		if err != nil {
			// terminal.readSecret accepts EOF-terminated passwords
			// if non-empty, so we do the same
			if errors.Is(err, io.EOF) && len(pw) > 0 {
				err = nil
			}
			return pw, err
		}
	}
}

func NewPasswordCbk() PasswordCallback {
	var pwdSet bool
	var pwd string
	return func() (string, error) {
		if pwdSet {
			return pwd, nil
		}

		p, err := readSecret("please enter password:")
		if err != nil {
			return "", err
		}
		pwdSet = true
		pwd = string(p)

		return pwd, err
	}
}

func NewPassPhraseCbk() PassPhraseCallback {
	var pwdSet bool
	var pwd string
	return func() (string, error) {
		if pwdSet {
			return pwd, nil
		}

		p, err := readSecret("please enter passphrase to private key:")
		if err != nil {
			return "", err
		}
		pwdSet = true
		pwd = string(p)

		return pwd, err
	}
}

func NewHostKeyCbk() HostKeyCallback {
	var trust []byte
	return func(hostPort string, pubKey ssh.PublicKey) error {
		if bytes.Equal(trust, pubKey.Marshal()) {
			return nil
		}
		msg := `The authenticity of host %s cannot be established.
%s key fingerprint is %s
Are you sure you want to continue connecting (yes/no)? `
		fmt.Fprintf(os.Stderr, msg, hostPort, pubKey.Type(), ssh.FingerprintSHA256(pubKey))
		reader := bufio.NewReader(os.Stdin)
		answer, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		answer = strings.TrimRight(answer, "\r\n")
		answer = strings.ToLower(answer)

		if answer == "yes" || answer == "y" {
			trust = pubKey.Marshal()
			fmt.Fprintf(os.Stderr, "To avoid this in future add following line into your ~/.ssh/known_hosts:\n%s %s %s\n",
				hostPort, pubKey.Type(), base64.StdEncoding.EncodeToString(trust))
			return nil
		}

		return errors.New("key rejected")
	}
}
