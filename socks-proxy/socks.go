package socksproxy

import (
	"context"
	"log"

	"github.com/rs/zerolog"
	"github.com/things-go/go-socks5"
)

type Config struct {
	ListenAddress string
	socks5.NameResolver
	socks5.AddressRewriter
}

func ListenAndServe(ctx context.Context, c *Config) error {
	l := zerolog.Ctx(ctx).With().
		Str("component", "socks").
		Logger()

	// Create a SOCKS5 server
	server := socks5.NewServer(
		socks5.WithLogger(
			socks5.NewLogger(
				log.New(l, "", 0),
			),
		),
		socks5.WithResolver(c.NameResolver),
		socks5.WithRewriter(c.AddressRewriter),
	)

	// Create SOCKS5 proxy on localhost port 8000
	network := "tcp"
	zerolog.Ctx(ctx).
		Info().
		Msgf("Listening on %s://%s", network, c.ListenAddress)

	return server.ListenAndServe(network, c.ListenAddress)
}
