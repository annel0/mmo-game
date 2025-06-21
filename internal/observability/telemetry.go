package observability

import (
	"context"
	"time"

	"github.com/annel0/mmo-game/internal/logging"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// InitTelemetry настраивает OTLP экспортер и устанавливает глобальный TracerProvider.
// Возвращает функцию shutdown, которую нужно вызвать при завершении приложения.
func InitTelemetry(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	// OTLP HTTP экспортер (по умолчанию localhost:4318)
	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, err
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exp),
		trace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	logging.Info("📡 OpenTelemetry инициализирован (OTLP → 4318, service=%s)", serviceName)

	shutdown := func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return tp.Shutdown(ctx)
	}
	return shutdown, nil
}
