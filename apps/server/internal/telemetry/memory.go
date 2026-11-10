package telemetry

import (
	"bytes"
	"context"
	"os"
	"runtime/metrics"
	"strconv"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const (
	AttrAllocBytes     = "mem.alloc_bytes"
	AttrHeapBytes      = "mem.heap_bytes"
	AttrHeapDeltaBytes = "mem.heap_delta_bytes"

	metricAllocs      = "/gc/heap/allocs:bytes"
	metricHeapObjects = "/memory/classes/heap/objects:bytes"
)

type memory struct {
	allocs uint64
	heap   uint64
}

var samplePool = sync.Pool{New: func() any {
	return &[2]metrics.Sample{{Name: metricAllocs}, {Name: metricHeapObjects}}
}}

func readMemory() memory {
	s := samplePool.Get().(*[2]metrics.Sample)
	defer samplePool.Put(s)
	metrics.Read(s[:])
	return memory{allocs: s[0].Value.Uint64(), heap: s[1].Value.Uint64()}
}

type memSpan struct {
	trace.Span
	start memory
	once  sync.Once
}

func (s *memSpan) End(opts ...trace.SpanEndOption) {
	s.once.Do(func() {
		end := readMemory()
		s.Span.SetAttributes(
			attribute.Int64(AttrAllocBytes, int64(end.allocs-s.start.allocs)),
			attribute.Int64(AttrHeapBytes, int64(end.heap)),
			attribute.Int64(AttrHeapDeltaBytes, int64(end.heap)-int64(s.start.heap)),
		)
	})
	s.Span.End(opts...)
}

func registerProcessMemory(mp metric.MeterProvider) error {
	if _, err := residentBytes(); err != nil {
		return nil
	}
	_, err := mp.Meter(Name).Int64ObservableGauge("process.memory.usage",
		metric.WithUnit("By"),
		metric.WithDescription("Resident set size of the process."),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			n, err := residentBytes()
			if err != nil {
				return err
			}
			o.Observe(n)
			return nil
		}),
	)
	return err
}

func residentBytes() (int64, error) {
	b, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, err
	}
	fields := bytes.Fields(b)
	if len(fields) < 2 {
		return 0, strconv.ErrSyntax
	}
	pages, err := strconv.ParseInt(string(fields[1]), 10, 64)
	if err != nil {
		return 0, err
	}
	return pages * int64(os.Getpagesize()), nil
}
