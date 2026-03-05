package otelutils

import (
	"context"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
)

// Create a Resource for this service
func otelResource(ctx context.Context, serviceName string) (*resource.Resource, error) {
	var res *resource.Resource
	attrs := []attribute.KeyValue{
		semconv.ServiceNameKey.String(serviceName),
		semconv.ServiceNamespaceKey.String("org.cyverse"),
	}
	instanceID, err := os.Hostname()
	if err != nil {
		newUUID, err := uuid.NewRandom()
		if err == nil {
			instanceID = newUUID.String()
		}
	}
	if instanceID != "" {
		attrs = append(attrs, semconv.ServiceInstanceIDKey.String(instanceID))
	}

	res, err = resource.New(ctx,
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithHost(),
		// all of WithProcess except owner, which needs cgo or a $USER env var
		resource.WithProcessPID(),
		resource.WithProcessExecutableName(),
		resource.WithProcessExecutablePath(),
		resource.WithProcessCommandArgs(),
		resource.WithProcessRuntimeName(),
		resource.WithProcessRuntimeVersion(),
		resource.WithProcessRuntimeDescription(),
		resource.WithContainer(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(attrs...))
	if err != nil {
		return nil, err
	}

	return res, nil
}

// Get a TracerProvider using OTLP as the exporter
func otlptracerpcTracerProvider(ctx context.Context, serviceName, url string) (*tracesdk.TracerProvider, error) {
	// Create the exporter
	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(url))
	if err != nil {
		return nil, err
	}

	res, err := otelResource(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	tp := tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exp),
		tracesdk.WithResource(res),
	)

	return tp, nil
}

// Get a TracerProvider using the OTEL_* environment variables to determine configuration. Currently, only supports the
// OTLP exporter. Jager also uses the OTLP exporter, but support for the "jaeger" exporter setting is retained for
// backward compatibility.
func TracerProviderFromEnv(ctx context.Context, serviceName string, onErr func(error)) func() {
	var (
		tracerProvider *tracesdk.TracerProvider
		err            error
	)

	otelTracesExporter := os.Getenv("OTEL_TRACES_EXPORTER")
	if otelTracesExporter == "" || otelTracesExporter == "none" {
		// No TracerProvider was created because OTEL_TRACES_EXPORTER wasn't set, or was set to none
		return func() {}
	}
	switch otelTracesExporter {
	case "jaeger":
		jaegerEndpoint := os.Getenv("OTEL_EXPORTER_JAEGER_ENDPOINT")
		if jaegerEndpoint == "" {
			onErr(errors.New("Jaeger set as OpenTelemetry trace exporter, but no Jaeger endpoint configured."))
			return func() {}
		} else {
			tracerProvider, err = otlptracerpcTracerProvider(ctx, serviceName, jaegerEndpoint)
			if err != nil {
				onErr(err)
				return func() {}
			}
		}
	case "otlp":
		otlpEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		if otlpEndpoint == "" {
			onErr(errors.New("OTLP set as OpenTelemetry trace exporter, but no OTLP endpoint configured."))
			return func() {}
		} else {
			tracerProvider, err = otlptracerpcTracerProvider(ctx, serviceName, otlpEndpoint)
			if err != nil {
				onErr(err)
				return func() {}
			}
		}
	default:
		onErr(errors.Errorf("Unknown OTEL_TRACES_EXPORTER type: %s", otelTracesExporter))
		return func() {}
	}

	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return shutdownTracerProviderFn(tracerProvider, ctx, onErr)
}

func shutdownTracerProviderFn(tracerProvider *tracesdk.TracerProvider, tracerContext context.Context, onErr func(error)) func() {
	return func() {
		ctx, cancel := context.WithTimeout(tracerContext, time.Second*5)
		defer cancel()
		if err := tracerProvider.Shutdown(ctx); err != nil {
			onErr(err)
		}
	}
}
