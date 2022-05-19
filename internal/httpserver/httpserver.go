package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/justinas/alice"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/sync/errgroup"

	"github.com/bakins/twirp-reflection/reflection"
)

type Config struct {
	Address string `kong:"default=127.0.0.1:8080"`
}

func (c Config) Build(ctx context.Context) (*Server, error) {
	return New(WithConfig(c))
}

type serverConfig struct {
	network string
	address string
}

type Option interface {
	apply(*serverConfig) error
}

type serverOptionFunc func(*serverConfig) error

func (f serverOptionFunc) apply(c *serverConfig) error {
	return f(c)
}

type serverOptions []Option

func (s serverOptions) apply(c *serverConfig) error {
	for _, o := range s {
		if err := o.apply(c); err != nil {
			return err
		}
	}

	return nil
}

type Server struct {
	chain      alice.Chain
	listener   atomic.Value
	mux        *http.ServeMux
	reflection *reflection.Server
	config     *serverConfig
}

func WithServerAddress(network string, address string) Option {
	return serverOptionFunc(func(c *serverConfig) error {
		switch network {
		case "udp", "unixgram":
			return fmt.Errorf("unsupported network %q", network)
		case "":
			return errors.New("network must not be empty")
		}

		if address == "" {
			return errors.New("address must not be empty")
		}

		c.network = network
		c.address = address

		return nil
	})
}

func WithConfig(c Config) Option {
	options := serverOptions{
		WithServerAddress("tcp", c.Address),
	}

	return options
}

// New creates a new HTTP server
func New(options ...Option) (*Server, error) {
	cfg := serverConfig{
		network: "tcp",
		address: "127.0.0.1:0",
	}

	for _, o := range options {
		if err := o.apply(&cfg); err != nil {
			return nil, fmt.Errorf("failed to create HTTP server %w", err)
		}
	}

	s := &Server{
		config:     &cfg,
		mux:        http.NewServeMux(),
		reflection: reflection.NewServer(),
	}

	s.RegisterService(s.reflection)

	s.AddMiddleware(func(next http.Handler) http.Handler {
		return h2c.NewHandler(next, &http2.Server{})
	})

	s.AddMiddleware(gziphandler.GzipHandler)

	return s, nil
}

// Handle adds a handler for the given pattern.
func (s *Server) Handle(pattern string, handler http.Handler) {
	s.mux.Handle(pattern, handler)
}

func (s *Server) Run(ctx context.Context) error {
	listener, err := net.Listen(s.config.network, s.config.address)
	if err != nil {
		return fmt.Errorf(
			"failed to listen %q %q %w",
			s.config.network,
			s.config.address,
			err,
		)
	}

	s.listener.Store(listener)

	svr := &http.Server{
		Handler: s.chain.Then(s.mux),
	}

	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		if err := svr.Serve(listener); err != nil {
			if err != http.ErrServerClosed {
				return err
			}
		}

		return nil
	})

	eg.Go(func() error {
		<-ctx.Done()
		// allow adjusting timeout?
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second*10)
		defer shutdownCancel()

		_ = svr.Shutdown(shutdownCtx)

		return nil
	})

	return eg.Wait()
}

// TODO: allow setting a matcher on middleware?
func (s *Server) AddMiddleware(middleware func(http.Handler) http.Handler) {
	s.chain = s.chain.Append(middleware)
}

// WaitForAddress waits until an address is assigned. Useful when generating
// a listening socket.
func (s *Server) WaitForAddress(ctx context.Context) (net.Addr, error) {
	// so cheesy
	t := time.NewTicker(time.Millisecond * 100)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-t.C:
			raw := s.listener.Load()
			if raw != nil {
				l, ok := raw.(net.Listener)
				if ok {
					return l.Addr(), nil
				}
			}
		}
	}
}

type TwirpServer interface {
	reflection.TwirpServer
	http.Handler
}

// RegisterService registers twirp service
func (s *Server) RegisterService(t TwirpServer) {
	s.mux.Handle(t.PathPrefix(), t)
	s.reflection.RegisterService(t)
}
