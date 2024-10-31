package socksproxy

import (
	"context"

	"github.com/rs/zerolog/log"
	"github.com/things-go/go-socks5"
	"github.com/things-go/go-socks5/statute"
)

var LoopbackRewriter, _ = RedirectTo("127.0.0.1:0")

type withTranslation statute.AddrSpec

// Rewrite implements socks5.AddressRewriter.
func (r withTranslation) Rewrite(ctx context.Context, request *socks5.Request) (context.Context, *statute.AddrSpec) {
	l := log.With().
		Str("component", "rewriter").
		Any("request", request).
		Logger()

	select {
	case <-ctx.Done():
		return ctx, nil
	default:
		ret := statute.AddrSpec(r)
		if ret.Port == 0 {
			ret.Port = request.DestAddr.Port
		}

		l.Debug().Any("address", ret).Msg("translated")
		return ctx, &ret
	}

}

func RedirectTo(addr string) (socks5.AddressRewriter, error) {
	ret, err := statute.ParseAddrSpec(addr)
	if err != nil {
		return nil, err
	}

	return withTranslation(ret), nil
}
