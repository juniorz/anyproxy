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
	"github.com/things-go/go-socks5"

	httpecho "github.com/juniorz/anyproxy/http-echo"
	socksproxy "github.com/juniorz/anyproxy/socks-proxy"
)

var (
	listenHost = flag.String("listen", "127.0.0.1", "listen on the given IP address.")
	socksPort  = flag.String("port", "8000", "the listen port for the SOCKS5 server. Use -port=0 to disable the SOCKS5 server.")
	httpPort   = flag.String("http-port", "8080", "the listen port for the HTTP server. Use -http-port=0 to disable the HTTP server.")
	httpsPort  = flag.String("https-port", "8443", "the listen port for the HTTPS server. Use -https-port=0 to disable the HTTPS server.")

	targetHost = flag.String("target", "", "the target address for ANY request through the SOCKS proxy. Use -target=0.0.0.0 to preserve the destination address in the original request. Use an empty target to proxy to the internal HTTP(S) server.")
	targetPort = flag.String("target-port", "0", "the target port for ANY request through the proxy. Use -target-port=0 to preserve the destination port in the original request.")
)

type Server interface {
	Shutdown(ctx context.Context) error
}

func main() {
	flag.Parse()
	configureLogging()

	// base context
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	config, err := proxyConfig()
	if err != nil {
		panic(err)
	}

	fmt.Println("Running, press CTRL+C to terminate...")

	socksServer, serveSOCKS, err := socksproxy.NewServerFor(ctx, config)
	if err != nil {
		panic(err)
	}

	httpAddr := net.JoinHostPort(*listenHost, *httpPort)
	httpServer, serveHTTP, err := httpecho.NewServerFor(ctx, httpAddr)
	if err != nil {
		panic(err)
	}

	httpsAddr := net.JoinHostPort(*listenHost, *httpsPort)
	httpsServer, serveHTTPS, err := httpecho.NewServerForHTTPS(ctx, httpsAddr)
	if err != nil {
		panic(err)
	}

	// serve proxies
	go serveSOCKS()
	go serveHTTP()
	go serveHTTPS()

	s := <-terminateSignal() // Wait for termination
	fmt.Printf("%s received. Terminating...\n", s)

	// cause := fmt.Errorf("%s received.", s)
	// _ = cause
	// cancel(cause) // immediately cancel all ongoing tasks

	// shutdown servers with a timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 10*time.Second)
	defer shutdownCancel()

	if err := socksServer.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("could not shutdown")
	}

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("could not shutdown")
	}

	if err := httpsServer.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("could not shutdown")
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
	allRewriters := make([]socks5.AddressRewriter, 0, 3)

	// use target-port=0 to provide a valid port with semantics "keep unchanged"
	if *targetPort == "" {
		panic("invalid -target-port=<empty>")
	}

	// <empty> targetHost proxies to the internal HTTP(S) server
	if *targetHost == "" {
		*targetHost = *listenHost

		// map to the internal HTTP(S) server ports
		if *targetPort == "0" {
			allRewriters = append(allRewriters,
				socksproxy.MapPort("80", *httpPort),
				socksproxy.MapPort("443", *httpsPort),
			)
		}
	}

	// only IP supported atm
	targetAddr := net.ParseIP(*targetHost)
	if targetAddr == nil {
		return nil, fmt.Errorf("target address is not an IP")
	}

	addrRewriter, err := socksproxy.RedirectTo(net.JoinHostPort(*targetHost, *targetPort))
	if err != nil {
		return nil, err
	}

	allRewriters = append(allRewriters, addrRewriter)

	return &socksproxy.Config{
		ListenAddress:   net.JoinHostPort(*listenHost, *socksPort),
		NameResolver:    socksproxy.ResolveTo(targetAddr),
		AddressRewriter: socksproxy.NewChain(allRewriters...),
	}, nil
}
