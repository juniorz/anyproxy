package socksproxy

import (
	"net"

	"github.com/things-go/go-socks5"
)

var (
	NoRewrite        = NewChain()               //no-op
	LoopbackRewriter = RewriteHost("127.0.0.1") // preserves port
)

func RedirectTo(hostPort string) (socks5.AddressRewriter, error) {
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return nil, err
	}

	addressRewrite := RewriteHost(host)
	if port == "0" { // preserves port
		return addressRewrite, nil
	}

	return NewChain(
		MapPort("0", port),
		addressRewrite,
	), nil
}
