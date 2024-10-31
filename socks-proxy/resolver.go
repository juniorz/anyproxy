package socksproxy

import (
	"context"
	"net"

	"github.com/rs/zerolog/log"
	"github.com/things-go/go-socks5"
)

var LoopbackResolver = ResolveTo(net.IPv4(127, 0, 0, 1))

type withResponse net.IP

// Resolve implements socks5.NameResolver.
func (r withResponse) Resolve(ctx context.Context, name string) (context.Context, net.IP, error) {
	l := log.With().
		Str("component", "resolver").
		Str("name", name).
		Any("response", r).
		Logger()

	select {
	case <-ctx.Done():
		return ctx, nil, ctx.Err()
	default:
		l.Debug().Msg("resolved")
		return ctx, net.IP(r), nil
	}
}

func ResolveTo(resp net.IP) socks5.NameResolver {
	return withResponse(resp)
}
