package httpecho

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"time"

	zlog "github.com/rs/zerolog/log"
)

type serveFn func()

func newServer(ctx context.Context) *http.Server {
	// TODO: Should I just use https://github.com/google/martian/blob/master/cmd/proxy/main.go instead?
	l := zlog.With().
		Str("component", "http").
		Logger()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		l.Info().
			Str("method", req.Method).
			Str("uri", req.RequestURI)

		fmt.Fprintf(w, "OK!")
	})

	return &http.Server{
		Handler: mux,

		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,

		ErrorLog: log.New(l.With().Str("level", "error").Logger(), "", 0),

		ConnState: func(c net.Conn, s http.ConnState) {
			l.Debug().
				Str("localAddr", c.LocalAddr().String()).
				Str("remoteAddr", c.RemoteAddr().String()).
				Str("state", s.String()).
				Msg("connection state change")
		},

		BaseContext: func(l net.Listener) context.Context {
			return ctx // forward the server context
		},
	}
}

func NewServerFor(ctx context.Context, hostPort string) (*http.Server, serveFn, error) {
	ll := zlog.With().
		Str("component", "http").
		Logger()

	server := newServer(ll.WithContext(ctx))

	return server, func() {
		ll.Info().Msgf("listening to HTTP server on %s", hostPort)

		l, err := net.Listen("tcp", hostPort)
		if err != nil {
			ll.Error().Err(err).Msg("failed to create listener for HTTP server")
		}

		if err := server.Serve(l); err != http.ErrServerClosed {
			ll.Error().Err(err).Msg("HTTP server terminated")
		}
	}, nil
}

func createCert(hosts []string) (tls.Certificate, error) {
	// https://go.dev/src/crypto/tls/generate_cert.go

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	// random serial number
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"anyproxy"},
		},

		// valid for 1 year
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),

		IsCA:                  true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		return tls.Certificate{}, err
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPem := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	})

	keyPem := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privBytes,
	})

	return tls.X509KeyPair(certPem, keyPem)
}

func NewServerForHTTPS(ctx context.Context, hostPort string) (*http.Server, serveFn, error) {
	ll := zlog.With().
		Str("component", "https").
		Logger()

	cert, err := createCert([]string{"127.0.0.1"}) // TODO: use targetAddr
	if err != nil {
		return nil, nil, err
	}

	server := newServer(ll.WithContext(ctx))

	return server, func() {
		ll.Info().Msgf("listening to HTTPS server on %s", hostPort)

		l, err := net.Listen("tcp", hostPort)
		if err != nil {
			ll.Error().Err(err).Msg("failed to create listener for HTTPS server")
		}

		tlsConfig := &tls.Config{ // TODO: probably move to a Config struct
			InsecureSkipVerify: true,
			Certificates:       []tls.Certificate{cert},
		}

		if server.Serve(tls.NewListener(l, tlsConfig)) != http.ErrServerClosed {
			ll.Error().Err(err).Msg("HTTPS server terminated")
		}
	}, nil
}
