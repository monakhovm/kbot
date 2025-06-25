// kbot-app/cmd/instrumentation.go
// Цей файл містить функції для ініціалізації OpenTelemetry TracerProvider та MeterProvider.

package cmd

import (
	"context"
	"log"
	"time"

	"go.opentelemetry.io/otel" // Додаємо імпорт codes
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

const (
	serviceName    = "kbot-app"
	serviceVersion = "1.0.0"                                            // Використовуємо appVersion, якщо він доступний глобально
	otlpEndpoint   = "otel-collector.monitoring.svc.cluster.local:4317" // OTLP gRPC endpoint для OpenTelemetry Collector
)

// InitTelemetry ініціалізує як MeterProvider, так і TracerProvider для OpenTelemetry.
// Вона повертає функцію, яку слід викликати для завершення роботи провайдерів.
func InitTelemetry() (func(), error) {
	ctx := context.Background()

	// Створення ресурсу для OTel
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(serviceVersion),
		),
	)
	if err != nil {
		log.Fatalf("Failed to create resource: %v", err)
	}

	// --- Ініціалізація TracerProvider (для трасування) ---
	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithInsecure(), // Використовувати тільки для розробки, не для production
		otlptracegrpc.WithEndpoint(otlpEndpoint),
		otlptracegrpc.WithTimeout(5*time.Second),
	)
	if err != nil {
		log.Fatalf("Failed to create trace exporter: %v", err)
	}

	bsp := trace.NewBatchSpanProcessor(traceExporter)
	tracerProvider := trace.NewTracerProvider(
		trace.WithResource(res),
		trace.WithSpanProcessor(bsp),
	)
	otel.SetTracerProvider(tracerProvider)

	// Налаштування глобального провайдера контексту для трасування
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// --- Ініціалізація MeterProvider (для метрик) ---
	metricExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithInsecure(), // Використовувати тільки для розробки, не для production
		otlpmetricgrpc.WithEndpoint(otlpEndpoint),
		// Змінено: WithMetricAggregationTemporalitySelector не є прямою опцією для New().
		// Це зазвичай налаштовується на рівні MeterProvider або через OTLP Exporter.
		// Якщо потрібно встановити агрегацію, це робиться через опції NewPeriodicReader або NewPushController.
		// Для OTLP/gRPC зазвичай використовується DeltaAggregationTemporalitySelector для Push.
		// Просто видаляємо цей рядок тут, оскільки він викликає помилку і не потрібен для базової роботи.
		otlpmetricgrpc.WithTimeout(5*time.Second),
	)
	if err != nil {
		log.Fatalf("Failed to create metric exporter: %v", err)
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(metricExporter, metric.WithInterval(10*time.Second))),
	)
	otel.SetMeterProvider(meterProvider)

	log.Printf("OpenTelemetry initialized. OTLP endpoint: %s", otlpEndpoint)

	// Функція для завершення роботи провайдерів
	return func() {
		cxt, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		if err := tracerProvider.Shutdown(cxt); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
		if err := meterProvider.Shutdown(cxt); err != nil {
			log.Printf("Error shutting down meter provider: %v", err)
		}
		log.Println("OpenTelemetry shut down.")
	}, nil
}
