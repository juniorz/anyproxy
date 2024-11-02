package socksproxy

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	zl "github.com/rs/zerolog/log"
	"github.com/things-go/go-socks5"
	"golang.org/x/exp/rand"
)

const shutdownPollIntervalMax = 500 * time.Millisecond

var (
	serverContextKeyName = "socks-server"
	ServerContextKey     = &serverContextKeyName // use pointer for efficiency

	ErrServerClosed = fmt.Errorf("socks: server closed")
)

type connState int

const (
	stateNew connState = iota
	stateActive
	stateClosed
)

type conn struct {
	net.Conn
	*server

	curState atomic.Uint64 // packed (unixtime<<8|uint8(ConnState))
}

func (c *conn) setState(state connState) {
	srv := c.server
	switch state {
	case stateNew:
		srv.trackConn(c, true)
	case stateClosed:
		srv.trackConn(c, false)
	}

	if state > 0xff || state < 0 {
		panic("internal error")
	}

	packedState := uint64(time.Now().Unix()<<8) | uint64(state)
	c.curState.Store(packedState)
}

func (c *conn) getState() (state connState, unixSec int64) {
	packedState := c.curState.Load()
	return connState(packedState & 0xff), int64(packedState >> 8)
}

func (c *conn) Read(b []byte) (n int, err error) {
	c.setState(stateActive)
	return c.Conn.Read(b)
}

func (c *conn) Write(b []byte) (n int, err error) {
	c.setState(stateActive)
	return c.Conn.Write(b)
}

func (c *conn) Close() error {
	defer c.setState(stateClosed)
	return c.Conn.Close()
}

type server struct {
	w   *socks5.Server
	log *zerolog.Logger

	l         net.Listener // store Listener for Close() and Shutdown()
	lErr      error        // its Close() error
	sync.Once              // its unique Close()

	inShutdown atomic.Bool // true when server is in shutdown

	activeConn map[*conn]struct{} // tracks connections
	sync.Mutex                    // its mutex
}

func (s *server) shuttingDown() bool {
	return s.inShutdown.Load()
}

func (s *server) newConn(c net.Conn) *conn {
	ret := &conn{
		Conn:   c,
		server: s,
	}

	ret.setState(stateNew)

	return ret
}

func (s *server) trackConn(c *conn, add bool) {
	s.Lock()
	defer s.Unlock()

	if s.activeConn == nil {
		s.activeConn = make(map[*conn]struct{})
	}

	if add {
		s.activeConn[c] = struct{}{}
	} else {
		delete(s.activeConn, c)
	}
}

func (s *server) closeIdleConns() bool {
	s.Lock()
	defer s.Unlock()
	quiescent := true

	// heuristic to give active (or recently created) connections enough time to complete
	for c := range s.activeConn {
		st, stateDurationSec := c.getState()

		// potentially untracked by the server
		if stateDurationSec == 0 {
			panic("attempted to close untracked connection")
		}

		// new connections are considered idle for a few seconds
		inactiveThreshold := int64(3)
		inactiveNew := st == stateNew && stateDurationSec < time.Now().Unix()-inactiveThreshold

		if !inactiveNew {
			quiescent = false
			continue
		}

		c.Conn.Close()
		delete(s.activeConn, c)
	}

	return quiescent
}

func (s *server) serveConn(ctx context.Context, conn net.Conn) {
	s.log.Debug().Msg("connection received")

	c := s.newConn(conn)

	go func() {
		// TODO: add context.Context to allow cancellations
		if err := s.w.ServeConn(c); err != nil {
			s.log.Err(err).Msg("connection error")
		}
	}()
}

func (s *server) Serve(l net.Listener) error {
	defer s.closeAndClearListener(l) // Serve() is responsible for clearing

	// shutting down: invalid state
	if s.shuttingDown() {
		return ErrServerClosed
	}

	s.Lock()
	s.l = l
	s.Unlock()

	s.log.Info().Msg("ready")
	s.log.Debug().Msg("accepting requests")
	for {
		conn, err := s.l.Accept()
		if err != nil {
			if s.shuttingDown() {
				return ErrServerClosed
			}

			return err
		}

		ctx := context.WithValue(context.Background(), ServerContextKey, s)
		go s.serveConn(ctx, conn)
	}
}

// requires lock!
func (s *server) closeListener() error {
	s.Once.Do(func() {
		s.lErr = s.l.Close()
	})

	return s.lErr
}

func (s *server) closeAndClearListener(l net.Listener) error {
	s.Lock()
	defer s.Unlock()

	err := s.closeListener()
	s.l = nil
	return err
}

func (s *server) Close() error {
	s.inShutdown.Store(true)
	s.Lock()
	defer s.Unlock()
	err := s.closeListener()

	for c := range s.activeConn { // immediately closes all active connections
		c.Conn.Close()
		delete(s.activeConn, c)
	}

	return err
}

func (s *server) Shutdown(ctx context.Context) error {
	s.inShutdown.Store(true)

	s.log.Debug().Msg("shutting down")

	s.Lock()
	err := s.closeListener()
	s.Unlock()

	// retry closing connections at increasing interval
	pollIntervalBase := time.Millisecond
	nextPollInterval := func() time.Duration {
		// Add 10% jitter.
		interval := pollIntervalBase + time.Duration(rand.Intn(int(pollIntervalBase/10)))
		// Double and clamp for next time.
		pollIntervalBase *= 2
		if pollIntervalBase > shutdownPollIntervalMax {
			pollIntervalBase = shutdownPollIntervalMax
		}
		return interval
	}

	timer := time.NewTimer(nextPollInterval())
	defer timer.Stop()
	for {
		if s.closeIdleConns() {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			timer.Reset(nextPollInterval())
		}
	}
}

type Config struct {
	ListenAddress string
	socks5.NameResolver
	socks5.AddressRewriter
}

func newServer(ctx context.Context, c *Config) *server {
	l := zl.Ctx(ctx)

	return &server{
		w: socks5.NewServer(
			socks5.WithLogger(
				socks5.NewLogger(
					log.New(l, "", 0),
				),
			),
			socks5.WithResolver(c.NameResolver),
			socks5.WithRewriter(c.AddressRewriter),
		),

		log: l, // ??
	}
}

func NewServerFor(ctx context.Context, c *Config) (*server, func(), error) {
	ll := zl.With().
		Str("component", "socks").
		Logger()

	server := newServer(ll.WithContext(ctx), c)

	return server, func() {
		ll.Info().Msgf("listening to SOCKS server on %s", c.ListenAddress)

		l, err := net.Listen("tcp", c.ListenAddress)
		if err != nil {
			ll.Error().Err(err).Msg("failed to create listener for SOCKS proxy")
		}

		if err := server.Serve(l); err != ErrServerClosed {
			ll.Error().Err(err).Msg("socks server terminated")
		}

	}, nil
}
