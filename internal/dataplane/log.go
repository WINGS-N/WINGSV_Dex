package dataplane

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func logLine(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintf(w, format+"\n", args...)
}

func stderrWriter(w io.Writer) io.Writer {
	if w == nil {
		return os.Stderr
	}
	return linePrefixWriter{prefix: "dataplane helper stderr: ", out: io.MultiWriter(os.Stderr, w)}
}

type linePrefixWriter struct {
	prefix string
	out    io.Writer
}

func (w linePrefixWriter) Write(p []byte) (int, error) {
	text := strings.ReplaceAll(string(p), "\r\n", "\n")
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		_, _ = fmt.Fprintln(w.out, w.prefix+line)
	}
	return len(p), nil
}
