package otel

import (
	"context"
	"fmt"
	"time"

	gcppropagator "github.com/GoogleCloudPlatform/opentelemetry-operations-go/propagator"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/propagation"
	controller "go.opentelemetry.io/otel/sdk/metric/controller/basic"
	processor "go.opentelemetry.io/otel/sdk/metric/processor/basic"
	"go.opentelemetry.io/otel/sdk/metric/selector/simple"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"

	"github.com/bakins/twirp-todo-example/internal/metadata"
)

type TraceConfig struct {
	Endpoint string `kong:""`
}

func (c TraceConfig) Build(ctx context.Context) (func(), error) {
	if c.Endpoint == "" {
		return func() {}, nil
	}

	exp, err := otlptracehttp.New(
		ctx,
		otlptracehttp.WithEndpoint(c.Endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter %w", err)
	}

	r, err := resource.New(
		ctx,
		resource.WithAttributes(
			attribute.String("service", metadata.Service()),
			attribute.String("version", metadata.Version()),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource %w", err)
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exp),
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithResource(r),
	)

	otel.SetTracerProvider(tp)

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			gcppropagator.CloudTraceFormatPropagator{},
			propagation.TraceContext{},
			propagation.Baggage{},
		))

	cleanup := func() {
		_ = tp.Shutdown(context.Background())
		_ = exp.Shutdown(context.Background())
	}
	return cleanup, nil
}

type MetricsConfig struct {
	Endpoint string `kong:""`
}

func (c MetricsConfig) Build(ctx context.Context) (func(), error) {
	if c.Endpoint == "" {
		return func() {}, nil
	}

	exp, err := otlpmetrichttp.New(
		ctx,
		otlpmetrichttp.WithEndpoint(c.Endpoint),
		otlpmetrichttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics exporter %w", err)
	}

	r, err := resource.New(
		ctx,
		resource.WithAttributes(
			attribute.String("service", metadata.Service()),
			attribute.String("version", metadata.Version()),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource %w", err)
	}

	pusher := controller.New(
		processor.NewFactory(
			simple.NewWithHistogramDistribution(),
			exp,
		),
		controller.WithExporter(exp),
		controller.WithCollectPeriod(time.Second),
		controller.WithResource(r),
	)

	if err := pusher.Start(context.Background()); err != nil {
		return nil, err
	}

	global.SetMeterProvider(pusher)

	cleanup := func() {
		if err := pusher.Stop(context.Background()); err != nil {
			otel.Handle(err)
		}

		if err := exp.Shutdown(context.Background()); err != nil {
			otel.Handle(err)
		}
	}

	return cleanup, nil
}
