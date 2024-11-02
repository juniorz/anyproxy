package socksproxy

import (
	"context"
	"net"

	"github.com/rs/zerolog/log"
	"github.com/things-go/go-socks5"
	"github.com/things-go/go-socks5/statute"
)

type hostTranslation net.IP

// Rewrite implements socks5.AddressRewriter.
func (host hostTranslation) Rewrite(ctx context.Context, request *socks5.Request) (context.Context, *statute.AddrSpec) {
	l := log.With().
		Str("component", "host-translate").
		Any("request", request).
		Logger()

	select {
	case <-ctx.Done():
		return ctx, request.DestAddr
	default:
		ret := &statute.AddrSpec{
			IP:   net.IP(host),
			Port: request.DestAddr.Port,
		}

		l.Debug().Any("address", ret).Msg("translated")
		return ctx, ret
	}
}

func RewriteHost(host string) socks5.AddressRewriter {
	return hostTranslation(net.ParseIP(host))
}
