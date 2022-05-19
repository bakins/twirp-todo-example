// Package app ties together the various packages to make a runnable server
package app

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/twitchtv/twirp"

	"github.com/bakins/twirp-reflection/reflection"
	"github.com/bakins/twirpotel"

	"github.com/bakins/twirp-todo-example/internal/database"
	"github.com/bakins/twirp-todo-example/internal/httpserver"
	"github.com/bakins/twirp-todo-example/internal/logging"
	"github.com/bakins/twirp-todo-example/internal/otel"
	pb "github.com/bakins/twirp-todo-example/internal/proto"
	"github.com/bakins/twirp-todo-example/internal/todo"
)

type Config struct {
	Logging    logging.Config     `kong:"embed,prefix=log."`
	Httpserver httpserver.Config  `kong:"embed,prefix=http."`
	Trace      otel.TraceConfig   `kong:"embed,prefix=trace."`
	Metrics    otel.MetricsConfig `kong:"embed,prefix=metrics."`
	Database   database.Config    `kong:"embed,prefix=database."`
}

// Main should be called from  main.main.
func Main() int {
	var cfg Config

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGQUIT)
	defer cancel()

	return logging.Exit(cfg.Run(ctx))
}

func (config Config) Run(ctx context.Context) error {
	logger := config.Logging.Build(ctx)
	defer logger.Sync()

	traceCleanup, err := config.Trace.Build(ctx)
	if err != nil {
		return err
	}

	defer traceCleanup()

	metricsCleanup, err := config.Metrics.Build(ctx)
	if err != nil {
		return err
	}

	defer metricsCleanup()

	db, err := config.Database.Build(ctx)
	if err != nil {
		return err
	}

	defer db.Close()

	svr, err := config.Httpserver.Build(ctx)
	if err != nil {
		return err
	}

	s, err := todo.New(db)
	if err != nil {
		return err
	}

	ts := pb.NewTodoServiceServer(
		s,
		twirp.WithServerInterceptors(
			twirpotel.ServerInterceptor(),
		),
	)

	svr.Handle(ts.PathPrefix(), ts)

	r := reflection.NewServer()
	r.RegisterService(ts)
	svr.Handle(r.PathPrefix(), r)

	return svr.Run(ctx)
}
