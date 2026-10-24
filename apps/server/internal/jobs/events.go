package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

type Attr struct {
	Key   string
	Value string
}

func EventAttrs(e Event) ([]Attr, error) {
	dec := json.NewDecoder(bytes.NewReader(e.Record))
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	var out []Attr
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, _ := tok.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		switch key {
		case slog.TimeKey, slog.LevelKey, slog.MessageKey:
			continue
		}
		out = append(out, Attr{Key: key, Value: attrValue(raw)})
	}
	return out, nil
}

func attrValue(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return strings.Join(list, ", ")
	}
	return string(raw)
}

type eventWriter struct {
	ctx      context.Context
	db       *store.Store
	job      int64
	now      func() time.Time
	fallback *slog.Logger
}

var _ io.Writer = (*eventWriter)(nil)

func (w *eventWriter) Write(p []byte) (int, error) {
	var head struct {
		Level slog.Level `json:"level"`
		Msg   string     `json:"msg"`
	}
	record := bytes.TrimSpace(p)
	err := json.Unmarshal(record, &head)
	if err == nil {
		err = w.db.AppendJobEvent(w.ctx, store.JobEvent{JobID: w.job, At: w.now(), Level: head.Level, Message: head.Msg, Record: record})
	}
	if err != nil {
		w.fallback.Error("record job event", "err", err)
		return 0, fmt.Errorf("record job event: %w", err)
	}
	return len(p), nil
}
