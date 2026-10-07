package logs

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestHandler(t *testing.T) {
	at := time.Date(2026, 10, 7, 20, 39, 27, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		color bool
		level slog.Level
		msg   string
		attrs []any
		want  string
	}{
		{
			name:  "plain",
			level: slog.LevelInfo,
			msg:   "site loaded",
			attrs: []any{"took", 7570 * time.Microsecond},
			want:  "20:39:27 INFO  site loaded took=7.57ms\n",
		},
		{
			name:  "quoted value",
			level: slog.LevelWarn,
			msg:   "failed to write response",
			attrs: []any{"err", "broken pipe"},
			want:  "20:39:27 WARN  failed to write response err=\"broken pipe\"\n",
		},
		{
			name:  "several lines",
			level: slog.LevelError,
			msg:   "GET /ai/\nsite/ai.mst: missing doc-meta\n  hint: start with #article",
			attrs: []any{"status", 500},
			want: "20:39:27 ERROR GET /ai/ status=500\n" +
				"               site/ai.mst: missing doc-meta\n" +
				"                 hint: start with #article\n",
		},
		{
			name:  "color",
			color: true,
			level: slog.LevelError,
			msg:   "failed",
			attrs: []any{"status", 500},
			want:  "\x1b[2m20:39:27\x1b[0m \x1b[31mERROR\x1b[0m failed \x1b[2mstatus=\x1b[0m500\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			h := NewHandler(&buf, slog.LevelInfo, tc.color)
			r := slog.NewRecord(at, tc.level, tc.msg, 0)
			r.Add(tc.attrs...)
			if err := h.Handle(context.Background(), r); err != nil {
				t.Fatalf("Handle() = %v", err)
			}
			if got := buf.String(); got != tc.want {
				t.Errorf("Handle() wrote\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestHandlerWithAttrsAndGroup(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(NewHandler(&buf, slog.LevelInfo, false)).With("a", 1).WithGroup("g").With("b", 2)
	l.Info("m", "c", 3)
	want := " m a=1 g.b=2 g.c=3\n"
	if got := buf.String(); len(got) < len(want) || got[len(got)-len(want):] != want {
		t.Errorf("logged %q, want it to end in %q", got, want)
	}
}

func TestHandlerLevel(t *testing.T) {
	var buf bytes.Buffer
	slog.New(NewHandler(&buf, slog.LevelInfo, false)).Debug("hidden")
	if buf.Len() != 0 {
		t.Errorf("Debug() below the level wrote %q", buf.String())
	}
}
