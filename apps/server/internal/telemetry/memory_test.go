package telemetry

import (
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestSpanMemoryAttributes(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	_, span := Start(t.Context(), "alloc")
	buf := make([][]byte, 0, 64)
	for range 64 {
		buf = append(buf, make([]byte, 64<<10))
	}
	End(span, nil)
	_ = buf

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	got := map[string]int64{}
	for _, kv := range spans[0].Attributes {
		got[string(kv.Key)] = kv.Value.AsInt64()
	}
	if got[AttrAllocBytes] < 64*64<<10 {
		t.Errorf("%s = %d, want at least %d", AttrAllocBytes, got[AttrAllocBytes], 64*64<<10)
	}
	if got[AttrHeapBytes] <= 0 {
		t.Errorf("%s = %d, want positive", AttrHeapBytes, got[AttrHeapBytes])
	}
	if _, ok := got[AttrHeapDeltaBytes]; !ok {
		t.Errorf("missing %s", AttrHeapDeltaBytes)
	}
}
