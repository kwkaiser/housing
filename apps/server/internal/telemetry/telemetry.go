package telemetry

import (
	"context"
	"database/sql/driver"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/XSAM/otelsql"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	Name        = "git.kwkaiser.io/kwkaiser/housing"
	ServiceName = "housing"
)

var tracer = otel.Tracer(Name)

type Telemetry struct {
	enabled  bool
	shutdown []func(context.Context) error
}

func Enabled() bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("OTEL_SDK_DISABLED")), "true") {
		return false
	}
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != ""
}

func Setup(ctx context.Context, version string) (*Telemetry, error) {
	t := &Telemetry{}
	if !Enabled() {
		return t, nil
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(ServiceName), semconv.ServiceVersion(version)),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
	)
	if err != nil {
		return nil, err
	}

	traces, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(traces), sdktrace.WithResource(res))
	t.shutdown = append(t.shutdown, tp.Shutdown)

	logs, err := otlploghttp.New(ctx)
	if err != nil {
		return nil, errors.Join(err, t.Shutdown(ctx))
	}
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewBatchProcessor(logs)), sdklog.WithResource(res))
	t.shutdown = append(t.shutdown, lp.Shutdown)

	otel.SetTracerProvider(tp)
	global.SetLoggerProvider(lp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	t.enabled = true
	return t, nil
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	var errs []error
	for i := len(t.shutdown) - 1; i >= 0; i-- {
		errs = append(errs, t.shutdown[i](ctx))
	}
	t.shutdown = nil
	return errors.Join(errs...)
}

func (t *Telemetry) Handler(h slog.Handler) slog.Handler {
	if t == nil || !t.enabled {
		return h
	}
	return slog.NewMultiHandler(h, otelslog.NewHandler(Name))
}

func Start(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

func End(span trace.Span, err error) {
	Record(span, err)
	span.End()
}

func Record(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

func Transport(rt http.RoundTripper) http.RoundTripper {
	if rt == nil {
		rt = http.DefaultTransport
	}
	return otelhttp.NewTransport(rt)
}

func Client(timeout time.Duration) *http.Client {
	return &http.Client{Transport: Transport(nil), Timeout: timeout}
}

func Server(h http.Handler, skip ...string) http.Handler {
	return otelhttp.NewHandler(h, ServiceName, otelhttp.WithFilter(func(r *http.Request) bool {
		for _, p := range skip {
			if r.URL.Path == p {
				return false
			}
		}
		return true
	}))
}

func WithSpan(log *slog.Logger, ctx context.Context) *slog.Logger {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return log
	}
	return slog.New(spanHandler{Handler: log.Handler(), sc: sc})
}

type spanHandler struct {
	slog.Handler
	sc trace.SpanContext
}

func (h spanHandler) Handle(ctx context.Context, r slog.Record) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if !trace.SpanContextFromContext(ctx).IsValid() {
		ctx = trace.ContextWithSpanContext(ctx, h.sc)
	}
	return h.Handler.Handle(ctx, r)
}

func (h spanHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return spanHandler{Handler: h.Handler.WithAttrs(attrs), sc: h.sc}
}

func (h spanHandler) WithGroup(name string) slog.Handler {
	return spanHandler{Handler: h.Handler.WithGroup(name), sc: h.sc}
}

func SQLDriver(name string) (string, error) {
	return otelsql.Register(name,
		otelsql.WithAttributes(semconv.DBSystemNameSQLite),
		otelsql.WithSpanOptions(otelsql.SpanOptions{
			DisableErrSkip:       true,
			OmitConnResetSession: true,
			OmitConnPrepare:      true,
			OmitRows:             true,
			OmitConnectorConnect: true,
			SpanFilter: func(ctx context.Context, _ otelsql.Method, _ string, _ []driver.NamedValue) bool {
				return trace.SpanContextFromContext(ctx).IsValid()
			},
		}),
	)
}
