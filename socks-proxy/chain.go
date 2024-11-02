package socksproxy

import (
	"context"

	"github.com/things-go/go-socks5"
	"github.com/things-go/go-socks5/statute"
)

type chain struct {
	r []socks5.AddressRewriter
}

// Rewrite implements socks5.AddressRewriter.
func (p *chain) Rewrite(ctx context.Context, request *socks5.Request) (context.Context, *statute.AddrSpec) {
	// no-op
	if p == nil {
		return ctx, request.DestAddr
	}

	req := *request // make a copy

	// Applies ALL rewriters in the chain in order
	for _, mapper := range p.r {
		// if mapper == nil {
		// 	continue
		// }

		ctx, req.DestAddr = mapper.Rewrite(ctx, &req)
	}

	return ctx, req.DestAddr
}

func NewChain(r ...socks5.AddressRewriter) socks5.AddressRewriter {
	switch len(r) {
	case 0:
		return nil
	case 1:
		return r[0]
	default:
		return &chain{r}
	}
}
