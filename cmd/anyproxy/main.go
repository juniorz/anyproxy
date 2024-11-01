package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	socksproxy "github.com/juniorz/anyproxy/socks-proxy"
)

var (
	listenHost = flag.String("listen", "127.0.0.1", "listen on the given IP address.")
	socksPort  = flag.String("port", "8000", "the listen port for the SOCKS5 server. Use -port=0 to disable the SOCKS5 server.")
	httpPort   = flag.String("http-port", "8080", "the listen port for the SOCKS5 server. Use -http-port=0 to disable the HTTP server.")

	targetHostPort = flag.String("target", "127.0.0.1:0", "the target address (host:port) for ANY request through the proxy. Use port=0 to preserve the destination port in the original request.")
)

func main() {
	flag.Parse()
	configureLogging()

	socksServer, err := socksProxyServer()
	if err != nil {
		panic(err)
	}

	go listenAndServeSOCKS(socksServer)

	// main context
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	fmt.Println("Running, press ctrl+c to terminate...")
	s := <-terminateSignal() // Wait for termination
	fmt.Printf("%s received. Terminating...\n", s)

	// cause := fmt.Errorf("%s received.", s)
	// _ = cause
	// cancel(cause) // immediately cancel all ongoing tasks

	// Ignore errors and shutdown servers with a timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 10*time.Second)
	defer shutdownCancel()

	if err := socksServer.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("could not shutdown")
	}
}

func listenAndServeSOCKS(socksServer socksproxy.Server) {
	hostPort := net.JoinHostPort(*listenHost, *socksPort)

	log.Info().Msgf("listening to SOCKS server on %s", hostPort)
	socksListener, err := net.Listen("tcp", hostPort)
	if err != nil {
		log.Error().Err(err).Msg("failed to create listener for SOCKS proxy")
	}

	if err := socksServer.Serve(socksListener); err != socksproxy.ErrServerClosed {
		log.Error().Err(err).Msg("socks server terminated")
	}
}

func terminateSignal() <-chan os.Signal {
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	return done
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
		ListenAddress:   net.JoinHostPort(*listenHost, *socksPort),
		NameResolver:    socksproxy.ResolveTo(targetAddr),
		AddressRewriter: rewriter,
	}, nil
}

func socksProxyServer() (socksproxy.Server, error) {
	config, err := proxyConfig()
	if err != nil {
		return nil, err
	}

	return socksproxy.NewServer(config), nil
}
