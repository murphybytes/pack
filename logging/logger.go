// Package logging defines the minimal interface that loggers must support to be used by pack.
package logging

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"sync"

	"github.com/buildpack/pack/style"
)

// Logger defines behavior required by a logging package used by pack libraries
type Logger interface {
	Debug(msg string)
	Debugf(fmt string, v ...interface{})

	Info(msg string)
	Infof(fmt string, v ...interface{})

	Warn(msg string)
	Warnf(fmt string, v ...interface{})

	Error(msg string)
	Errorf(fmt string, v ...interface{})

	Writer() io.Writer
}

// WithDebugErrorWriter is an optional interface for loggers that want to support a separate writer for errors and standard logging.
// the DebugErrorWriter should write to stderr if quiet is false.
type WithDebugErrorWriter interface {
	DebugErrorWriter() io.Writer
}

// WithDebugWriter is an optional interface what will return a writer that will write raw output if quiet is false.
type WithDebugWriter interface {
	DebugWriter() io.Writer
}

// GetDebugErrorWriter will return an ErrorWriter, typically stderr if one exists, otherwise the standard logger writer
// will be returned.
func GetDebugErrorWriter(l Logger) io.Writer {
	if er, ok := l.(WithDebugErrorWriter); ok {
		return er.DebugErrorWriter()
	}
	return l.Writer()
}

// GetDebugWriter returns a writer
// See WithDebugWriter
func GetDebugWriter(l Logger) io.Writer {
	if ew, ok := l.(WithDebugWriter); ok {
		return ew.DebugWriter()
	}
	return l.Writer()
}

// PrefixWriter will prefix writes
type PrefixWriter struct {
	sync.Mutex
	buffer           bytes.Buffer
	out              io.Writer
	prefix           string
	colorCodeMatcher *regexp.Regexp
}

// NewPrefixWriter produces writes that are prefixed and optionally stripped of ANSI color codes.
func NewPrefixWriter(w io.Writer, wantColor bool, prefix string) *PrefixWriter {
	pw := PrefixWriter{
		out:    w,
		prefix: fmt.Sprintf("[%s] ", style.Prefix(prefix)),
	}
	if !wantColor {
		pw.colorCodeMatcher = regexp.MustCompile(`\x1b\[[0-9;]*m`)
		pw.prefix = string(pw.colorCodeMatcher.ReplaceAll([]byte(pw.prefix), []byte("")))
	}
	return &pw
}

const lineFeed = "\n"

// Write buffers input, writing it to underlying writer when a line feed in encountered.
func (w *PrefixWriter) Write(buf []byte) (int, error) {
	w.Lock()
	defer w.Unlock()
	var err error
	n, _ := w.buffer.Write(buf)

	if bytes.HasSuffix(buf, []byte(lineFeed)) {
		contents := w.buffer.Bytes()
		w.buffer.Reset()

		if w.colorCodeMatcher != nil {
			contents = w.colorCodeMatcher.ReplaceAll(contents, []byte(""))
		}
		_, err = fmt.Fprint(w.out, w.prefix+string(contents))
	}

	return n, err
}

// Close writes any partial buffer data.
func (w *PrefixWriter) Close() error {
	w.Lock()
	defer w.Unlock()
	if w.buffer.Len() == 0 {
		return nil
	}
	contents := w.buffer.Bytes()
	w.buffer.Reset()

	if w.colorCodeMatcher != nil {
		contents = w.colorCodeMatcher.ReplaceAll(contents, []byte(""))
	}
	_, err := fmt.Fprintln(w.out, w.prefix+string(contents))
	return err
}

// Tip logs a tip.
func Tip(l Logger, format string, v ...interface{}) {
	l.Infof(style.Tip("Tip: ")+format, v...)
}
