// Package logging builds the service's slog logger: JSON to stdout always, and
// to an OpenTelemetry collector (e.g. Grafana Loki) when an OTLP endpoint is configured.
package logging

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Options configures New.
type Options struct {
	Level string
	// Format is "json" (default) or "text" for stdout.
	Format string
	// ServiceNamespace groups the service in your log backend, e.g. "acme-reviews-service".
	ServiceNamespace string
	// OTLPEndpoint, e.g. "http://otel-collector:4318"; empty = stdout only.
	OTLPEndpoint string
}

// New returns the logger and a shutdown function that flushes buffered logs.
func New(ctx context.Context, opts Options) (*slog.Logger, func(context.Context) error, error) {
	var stdout slog.Handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLevel(opts.Level)})
	if strings.EqualFold(opts.Format, "text") {
		stdout = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: parseLevel(opts.Level)})
	}
	if opts.OTLPEndpoint == "" {
		return slog.New(stdout), func(context.Context) error { return nil }, nil
	}

	exporter, err := otlploghttp.New(ctx, otlploghttp.WithEndpointURL(strings.TrimRight(opts.OTLPEndpoint, "/")+"/v1/logs"))
	if err != nil {
		return nil, nil, err
	}
	provider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
		sdklog.WithResource(resource.NewWithAttributes(semconv.SchemaURL,
			semconv.ServiceNamespace(opts.ServiceNamespace),
			semconv.ServiceName(opts.ServiceNamespace+"-web"),
		)),
	)
	otel := otelslog.NewHandler(opts.ServiceNamespace, otelslog.WithLoggerProvider(provider))
	return slog.New(fanout{stdout, otel}), provider.Shutdown, nil
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// fanout sends every record to all handlers (stdout and OpenTelemetry).
type fanout []slog.Handler

func (f fanout) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range f {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (f fanout) Handle(ctx context.Context, record slog.Record) error {
	var errs []error
	for _, handler := range f {
		if handler.Enabled(ctx, record.Level) {
			errs = append(errs, handler.Handle(ctx, record.Clone()))
		}
	}
	return errors.Join(errs...)
}

func (f fanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make(fanout, len(f))
	for i, handler := range f {
		out[i] = handler.WithAttrs(attrs)
	}
	return out
}

func (f fanout) WithGroup(name string) slog.Handler {
	out := make(fanout, len(f))
	for i, handler := range f {
		out[i] = handler.WithGroup(name)
	}
	return out
}
