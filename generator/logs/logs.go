// Package logs formats log records for a person watching a terminal: one line
// per record, with the time, the level, the message and the attributes, in
// color when the terminal takes it.
package logs

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// ANSI escape sequences.
const (
	reset  = "\x1b[0m"
	dim    = "\x1b[2m"
	red    = "\x1b[31m"
	yellow = "\x1b[33m"
	blue   = "\x1b[34m"
)

// Handler writes each record as "15:04:05 LEVEL message key=value ...". A
// message spanning several lines has the attributes on its first line and its
// other lines indented under it.
type Handler struct {
	mu    *sync.Mutex
	w     io.Writer
	color bool
	level slog.Level
	attrs string // preformatted attributes from WithAttrs
	group string // prefix of attribute keys from WithGroup
}

// NewHandler returns a handler writing records at level and above to w, in
// color if color is true.
func NewHandler(w io.Writer, level slog.Level, color bool) *Handler {
	return &Handler{mu: new(sync.Mutex), w: w, level: level, color: color}
}

// Color reports whether f is a terminal that takes color. The NO_COLOR
// convention turns color off.
func Color(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// Enabled reports whether h writes records at level.
func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle writes r.
func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(h.paint(dim, r.Time.Format(time.TimeOnly)))
	b.WriteByte(' ')
	b.WriteString(h.paint(levelColor(r.Level), fmt.Sprintf("%-5s", r.Level)))
	b.WriteByte(' ')

	head, rest, _ := strings.Cut(r.Message, "\n")
	b.WriteString(head)
	b.WriteString(h.attrs)
	r.Attrs(func(a slog.Attr) bool {
		b.WriteString(h.attr(a))
		return true
	})
	b.WriteByte('\n')
	// The other lines start under the message, past time and level.
	const indent = "               " // len("15:04:05 LEVEL ")
	for line := range strings.Lines(rest) {
		b.WriteString(indent + line)
	}
	if rest != "" && !strings.HasSuffix(rest, "\n") {
		b.WriteByte('\n')
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

// WithAttrs returns a handler that writes attrs with every record.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h2 := *h
	for _, a := range attrs {
		h2.attrs += h.attr(a)
	}
	return &h2
}

// WithGroup returns a handler that prefixes attribute keys with name.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := *h
	h2.group += name + "."
	return &h2
}

// attr returns a formatted as " key=value", or "" for an empty attribute.
func (h *Handler) attr(a slog.Attr) string {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return ""
	}
	v := a.Value.String()
	if a.Value.Kind() == slog.KindDuration {
		v = a.Value.Duration().Round(10 * time.Microsecond).String()
	}
	if strings.ContainsAny(v, " \t\n\"=") {
		v = fmt.Sprintf("%q", v)
	}
	return " " + h.paint(dim, h.group+a.Key+"=") + v
}

// paint returns s in color c if h writes color.
func (h *Handler) paint(c, s string) string {
	if !h.color {
		return s
	}
	return c + s + reset
}

func levelColor(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return red
	case l >= slog.LevelWarn:
		return yellow
	case l >= slog.LevelInfo:
		return blue
	default:
		return dim
	}
}
