package main

import (
	"context"
	"flag"
	"fmt"
	"net"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	socksproxy "github.com/juniorz/anyproxy/socks-proxy"
)

var (
	listenAddress  = flag.String("listen", "127.0.0.1:8000", "the listen address for the SOCKS5 server.")
	targetHostPort = flag.String("target", "127.0.0.1:0", "the target address (host:port) for ANY request through the proxy. Use port=0 to preserve the original destination port.")
)

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	configureLogging()

	if err := startSocksProxy(ctx); err != nil {
		panic(err)
	}
}

func configureLogging() {
	// UNIX Time is faster and smaller than most timestamps
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
}

func proxyConfig() (*socksproxy.Config, error) {
	host, _, err := net.SplitHostPort(*targetHostPort)
	if err != nil {
		return nil, err
	}

	targetAddr := net.ParseIP(host)
	if targetAddr == nil {
		return nil, fmt.Errorf("target address is not an IP")
	}

	rewriter, err := socksproxy.RedirectTo(*targetHostPort)
	if err != nil {
		return nil, err
	}

	return &socksproxy.Config{
		ListenAddress:   *listenAddress,
		NameResolver:    socksproxy.ResolveTo(targetAddr),
		AddressRewriter: rewriter,
	}, nil
}

// Starts the SOCKS5
func startSocksProxy(ctx context.Context) error {
	c := log.With().
		Str("component", "socks").
		Logger().
		WithContext(ctx)

	config, err := proxyConfig()
	if err != nil {
		return err
	}

	return socksproxy.ListenAndServe(c, config)
}
