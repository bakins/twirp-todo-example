package logging

import (
	"context"
	"errors"
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/bakins/twirp-todo-example/internal/metadata"
	"github.com/bakins/twirp-todo-example/internal/stackdriver"
)

type Config struct{}

func (c Config) Build(ctx context.Context) *zap.Logger {
	enc := stackdriver.Encoder()

	core := zapcore.NewCore(enc, Stdout, zap.NewAtomicLevelAt(zap.InfoLevel))

	wrapped := stackdriver.WrapCore(core, metadata.Service(), metadata.Version())

	logger := zap.New(
		wrapped,
		zap.ErrorOutput(Stderr),
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)

	zap.ReplaceGlobals(logger)

	return logger
}

// LoggingError wraps an error with a logger
type LoggingError struct {
	err     error
	logger  *zap.Logger
	message string
}

func (l *LoggingError) Error() string {
	return fmt.Sprintf("%s %v", l.message, l.err)
}

// Create a new logging error
func NewLoggingError(logger *zap.Logger, message string, err error) *LoggingError {
	l := LoggingError{
		err:     err,
		logger:  logger,
		message: message,
	}

	return &l
}

// Exit returns an exit code.
// if err is nil, 0 is returned.
// If err is set, the error is logged.
func Exit(err error) int {
	if err == nil {
		return 0
	}
	var le *LoggingError
	if errors.As(err, &le) {
		le.logger.Error(le.message, zap.Error(err))
		return 1
	}

	logger := zap.L()
	logger.Error("exit", zap.Error(err))
	return 1
}

var (
	Stdout = zapcore.AddSync(os.Stdout)
	Stderr = zapcore.AddSync(os.Stderr)
)

type ctxMarker struct{}

var (
	ctxMarkerKey = &ctxMarker{}
	nullLogger   = zap.NewNop()
)

// AddFields adds zap fields to the logger.
func AddFields(ctx context.Context, fields ...zapcore.Field) context.Context {
	l, ok := ctx.Value(ctxMarkerKey).(*zap.Logger)
	if !ok || l == nil {
		l = nullLogger
	}

	l = l.With(fields...)

	return ToContext(ctx, l)
}

func FromContext(ctx context.Context) *zap.Logger {
	l, ok := ctx.Value(ctxMarkerKey).(*zap.Logger)
	if !ok || l == nil {
		return nullLogger
	}

	return l
}

func ToContext(ctx context.Context, l *zap.Logger) context.Context {
	if l == nil {
		l = nullLogger
	}
	return context.WithValue(ctx, ctxMarkerKey, l)
}

// Debug is equivalent to calling Debug on the zap.Logger in the context.
// It is a no-op if the context does not contain a zap.Logger.
func Debug(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Debug(msg, fields...)
}

// Info is equivalent to calling Info on the zap.Logger in the context.
// It is a no-op if the context does not contain a zap.Logger.
func Info(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Info(msg, fields...)
}

// Warn is equivalent to calling Warn on the zap.Logger in the context.
// It is a no-op if the context does not contain a zap.Logger.
func Warn(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Warn(msg, fields...)
}

// Error is equivalent to calling Error on the zap.Logger in the context.
// It is a no-op if the context does not contain a zap.Logger.
func Error(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Error(msg, fields...)
}
