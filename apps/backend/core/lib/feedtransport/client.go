// Package feedtransport provides guarded network transports for catalogue plugins.
package feedtransport

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/skeletg/skelet/apps/backend/core/lib/urlguard"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

type Client struct {
	*http.Client
	dialer *net.Dialer
}

func New(allowPrivate bool) *Client {
	return &Client{Client: &http.Client{
		Transport:     urlguard.SafeTransport(allowPrivate),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, dialer: urlguard.SafeDialer(allowPrivate)}
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if _, err := urlguard.CheckShape(req.URL.String()); err != nil {
		return nil, sdk.Permanent(err)
	}
	return c.Client.Do(req)
}

var errHostKey = errors.New("SFTP host key does not match the pinned SHA256 fingerprint")

func (c *Client) UploadSFTP(ctx context.Context, target sdk.SFTPTarget, body io.Reader) error {
	if _, _, err := net.SplitHostPort(target.Address); err != nil || target.Username == "" || target.Path == "" || !strings.HasPrefix(target.HostKeySHA256, "SHA256:") {
		return sdk.Permanent(fmt.Errorf("SFTP requires host:port, username, path and a SHA256 host-key fingerprint"))
	}
	var auth ssh.AuthMethod
	switch target.AuthMethod {
	case "", "password":
		auth = ssh.Password(target.Secret)
	case "private_key":
		key, err := ssh.ParsePrivateKey([]byte(target.Secret))
		if err != nil {
			return sdk.Permanent(fmt.Errorf("invalid SFTP private key: %w", err))
		}
		auth = ssh.PublicKeys(key)
	default:
		return sdk.Permanent(fmt.Errorf("unsupported SFTP authentication method"))
	}
	conn, err := c.dialer.DialContext(ctx, "tcp", target.Address)
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	// Bound the SSH negotiation independently of a potentially long catalogue upload.
	if err := conn.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		return err
	}
	cc, chans, reqs, err := ssh.NewClientConn(conn, target.Address, &ssh.ClientConfig{
		User: target.Username, Auth: []ssh.AuthMethod{auth},
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			if subtle.ConstantTimeCompare([]byte(ssh.FingerprintSHA256(key)), []byte(target.HostKeySHA256)) != 1 {
				return errHostKey
			}
			return nil
		},
	})
	if err != nil {
		if errors.Is(err, errHostKey) {
			return sdk.Permanent(err)
		}
		return err
	}
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		_ = cc.Close()
		return err
	}
	sshClient := ssh.NewClient(cc, chans, reqs)
	defer sshClient.Close()
	client, err := sftp.NewClient(sshClient)
	if err != nil {
		return err
	}
	defer client.Close()
	tmp := path.Join(path.Dir(target.Path), "."+path.Base(target.Path)+"."+uuid.NewString()+".tmp")
	file, err := client.Create(tmp)
	if err != nil {
		return err
	}
	defer func() { _ = client.Remove(tmp) }()
	_, copyErr := io.Copy(file, body)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if _, ok := client.HasExtension("posix-rename@openssh.com"); ok {
		return client.PosixRename(tmp, target.Path)
	}
	return client.Rename(tmp, target.Path)
}
