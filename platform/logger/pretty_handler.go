package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ansiReset  = "\x1b[0m"
	ansiGray   = "\x1b[90m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiCyan   = "\x1b[36m"
	ansiBold   = "\x1b[1m"
)

type PrettyHandlerOptions struct {
	Level     slog.Leveler
	AddSource bool
	Color     bool
}

type attrChunk struct {
	groups []string
	attrs  []slog.Attr
}

type prettyHandler struct {
	out      io.Writer
	opts     PrettyHandlerOptions
	groups   []string
	chunks   []attrChunk
	writeMux *sync.Mutex
}

func NewPrettyHandler(out io.Writer, opts PrettyHandlerOptions) slog.Handler {
	if out == nil {
		out = io.Discard
	}
	return &prettyHandler{
		out:      out,
		opts:     opts,
		writeMux: &sync.Mutex{},
	}
}

func (h *prettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return level >= minLevel
}

func (h *prettyHandler) Handle(_ context.Context, rec slog.Record) error {
	var b strings.Builder

	t := rec.Time
	if t.IsZero() {
		t = time.Now()
	}
	timeText := t.Format("2006-01-02 15:04:05.000")
	levelText := strings.ToUpper(rec.Level.String())
	msgText := rec.Message
	if msgText == "" {
		msgText = "-"
	}

	if h.opts.Color {
		b.WriteString(ansiGray)
		b.WriteString(timeText)
		b.WriteString(ansiReset)
		b.WriteString(" ")
		b.WriteString(colorizeLevel(levelText, rec.Level))
		b.WriteString(" ")
		b.WriteString(ansiBold)
		b.WriteString(msgText)
		b.WriteString(ansiReset)
	} else {
		b.WriteString(timeText)
		b.WriteString(" ")
		b.WriteString(padLevel(levelText))
		b.WriteString(" ")
		b.WriteString(msgText)
	}

	parts := make([]string, 0, 16)
	for _, chunk := range h.chunks {
		for _, attr := range chunk.attrs {
			appendAttr(&parts, attr, chunk.groups, h.opts.Color)
		}
	}
	rec.Attrs(func(attr slog.Attr) bool {
		appendAttr(&parts, attr, h.groups, h.opts.Color)
		return true
	})
	if h.opts.AddSource && rec.PC != 0 {
		frames := runtime.CallersFrames([]uintptr{rec.PC})
		frame, _ := frames.Next()
		source := filepath.Base(frame.File) + ":" + strconv.Itoa(frame.Line)
		parts = append(parts, formatPair("source", source, h.opts.Color))
	}
	if len(parts) > 0 {
		b.WriteString(" | ")
		b.WriteString(strings.Join(parts, " "))
	}
	b.WriteString("\n")

	h.writeMux.Lock()
	defer h.writeMux.Unlock()
	_, err := io.WriteString(h.out, b.String())
	return err
}

func (h *prettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	copied := make([]slog.Attr, len(attrs))
	copy(copied, attrs)

	next := *h
	next.groups = append([]string(nil), h.groups...)
	next.chunks = append([]attrChunk(nil), h.chunks...)
	next.chunks = append(next.chunks, attrChunk{
		groups: append([]string(nil), h.groups...),
		attrs:  copied,
	})
	return &next
}

func (h *prettyHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := *h
	next.groups = append([]string(nil), h.groups...)
	next.groups = append(next.groups, name)
	next.chunks = append([]attrChunk(nil), h.chunks...)
	return &next
}

func appendAttr(parts *[]string, attr slog.Attr, groups []string, color bool) {
	if attr.Equal(slog.Attr{}) {
		return
	}
	if attr.Value.Kind() == slog.KindGroup {
		subGroup := append([]string(nil), groups...)
		if attr.Key != "" {
			subGroup = append(subGroup, attr.Key)
		}
		for _, ga := range attr.Value.Group() {
			appendAttr(parts, ga, subGroup, color)
		}
		return
	}
	if attr.Key == "" {
		return
	}
	key := attr.Key
	if len(groups) > 0 {
		key = strings.Join(append(append([]string(nil), groups...), attr.Key), ".")
	}
	*parts = append(*parts, formatPair(key, formatValue(attr.Value), color))
}

func formatPair(key, value string, color bool) string {
	if color {
		return ansiCyan + key + ansiReset + "=" + value
	}
	return key + "=" + value
}

func formatValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		return quoteIfNeeded(v.String())
	case slog.KindInt64:
		return strconv.FormatInt(v.Int64(), 10)
	case slog.KindUint64:
		return strconv.FormatUint(v.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.FormatFloat(v.Float64(), 'f', -1, 64)
	case slog.KindBool:
		return strconv.FormatBool(v.Bool())
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().Format(time.RFC3339Nano)
	case slog.KindAny:
		return quoteIfNeeded(fmt.Sprintf("%v", v.Any()))
	default:
		return quoteIfNeeded(v.String())
	}
}

func quoteIfNeeded(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, " \t\r\n=\"") {
		return strconv.Quote(s)
	}
	return s
}

func colorizeLevel(level string, lv slog.Level) string {
	color := ansiGreen
	switch {
	case lv >= slog.LevelError:
		color = ansiRed
	case lv >= slog.LevelWarn:
		color = ansiYellow
	case lv < slog.LevelInfo:
		color = ansiGray
	}
	return color + padLevel(level) + ansiReset
}

func padLevel(level string) string {
	l := strings.ToUpper(strings.TrimSpace(level))
	if len(l) >= 5 {
		return l
	}
	return l + strings.Repeat(" ", 5-len(l))
}
