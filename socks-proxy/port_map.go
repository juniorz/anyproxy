package socksproxy

import (
	"context"
	"strconv"

	"github.com/rs/zerolog/log"
	"github.com/things-go/go-socks5"
	"github.com/things-go/go-socks5/statute"
)

type portMapper struct {
	from, to int
}

// Rewrite implements socks5.AddressRewriter.
func (p *portMapper) Rewrite(ctx context.Context, request *socks5.Request) (context.Context, *statute.AddrSpec) {
	l := log.With().
		Str("component", "port-map").
		Any("request", request).
		Logger()

	select {
	case <-ctx.Done():
		return ctx, request.DestAddr
	default:
		// skip
		if p.from > 0 && request.DestAddr.Port != p.from {
			return ctx, request.DestAddr
		}

		// Preserves original destination address
		ret := statute.AddrSpec(*request.DestAddr)

		// map port
		if p.to > 0 {
			ret.Port = p.to
		}

		l.Debug().Any("address", ret).Msg("translated")
		return ctx, &ret
	}
}

func MapPort(from, to string) socks5.AddressRewriter {
	if from == "0" && to == "0" { // noop
		return NoRewrite
	}

	fromPort, _ := strconv.Atoi(from)
	toPort, _ := strconv.Atoi(to)
	return &portMapper{from: fromPort, to: toPort}
}
