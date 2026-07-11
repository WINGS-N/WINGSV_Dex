package applog

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const MaxFileBytes = 256 * 1024

// trimTarget is how far down a file is cut once it crosses MaxFileBytes. Trimming past the
// cap (not to it) means the next append does not immediately trip the cap again, so the
// expensive read+rewrite runs about once per (MaxFileBytes-trimTarget) of new data instead
// of on every line.
const trimTarget = MaxFileBytes * 3 / 4

const maxLineBytes = 8 * 1024

const (
	ChannelRuntime = "runtime"
	ChannelProxy   = "proxy"
)

type Store struct {
	dir      string
	versions map[string]uint64
	listener func(channel, line string)
	mu       sync.Mutex
}

// SetListener registers a callback invoked with each appended (redacted, final) line, so
// the caller can push it to the UI live. Set once at startup.
func (s *Store) SetListener(fn func(channel, line string)) {
	s.mu.Lock()
	s.listener = fn
	s.mu.Unlock()
}

type Snapshot struct {
	Channel string   `json:"channel"`
	Lines   []string `json:"lines"`
	Text    string   `json:"text"`
	Version uint64   `json:"version"`
}

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir, versions: map[string]uint64{}}, nil
}

func (s *Store) Snapshot(channel string) (Snapshot, error) {
	name, err := channelFile(channel)
	if err != nil {
		return Snapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	version := s.versions[channel]
	raw, err := os.ReadFile(filepath.Join(s.dir, name))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, err
	}
	text := string(raw)
	lines := splitLines(text)
	return Snapshot{Channel: channel, Lines: lines, Text: text, Version: version}, nil
}

func (s *Store) Append(channel string, line string) error {
	name, err := channelFile(channel)
	if err != nil {
		return err
	}
	line = Redact(line)
	if !looksTimestamped(line) {
		line = time.Now().UTC().Format(time.RFC3339Nano) + " " + line
	}
	line = truncateBytes(line, maxLineBytes)
	s.mu.Lock()
	path := filepath.Join(s.dir, name)
	if err := appendLine(path, line); err != nil {
		s.mu.Unlock()
		return err
	}
	s.versions[channel]++
	listener := s.listener
	s.mu.Unlock()
	if listener != nil {
		listener(channel, line)
	}
	return nil
}

func (s *Store) Clear(channel string) error {
	name, err := channelFile(channel)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.WriteFile(filepath.Join(s.dir, name), nil, 0o600); err != nil {
		return err
	}
	s.versions[channel]++
	return nil
}

func channelFile(channel string) (string, error) {
	switch channel {
	case ChannelRuntime:
		return "runtime.log", nil
	case ChannelProxy:
		return "proxy.log", nil
	default:
		return "", errors.New("unknown channel")
	}
}

func appendLine(path string, line string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(f, line+"\n"); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return boundFile(path)
}

func boundFile(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if fi.Size() <= MaxFileBytes {
		return nil // cheap common path: no read/rewrite until the cap is crossed
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := splitLines(string(raw))
	kept := make([]string, 0, len(lines))
	var size int
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		need := len(line) + 1
		if size+need > trimTarget && len(kept) > 0 {
			break
		}
		if need > trimTarget {
			line = truncateHeadBytes(line, trimTarget-1)
			need = len(line) + 1
		}
		kept = append(kept, line)
		size += need
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	text := strings.Join(kept, "\n")
	if len(kept) > 0 {
		text += "\n"
	}
	return os.WriteFile(path, []byte(text), 0o600)
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// truncateBytes trims s to at most max bytes without splitting the trailing UTF-8 rune.
func truncateBytes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	s = s[:max]
	for len(s) > 0 {
		if r, size := utf8.DecodeLastRuneInString(s); r == utf8.RuneError && size <= 1 {
			s = s[:len(s)-1]
			continue
		}
		break
	}
	return s
}

// truncateHeadBytes keeps at most max trailing bytes of s, dropping any leading partial
// UTF-8 rune so the kept tail stays valid.
func truncateHeadBytes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	s = s[len(s)-max:]
	for len(s) > 0 && !utf8.RuneStart(s[0]) {
		s = s[1:]
	}
	return s
}

func looksTimestamped(s string) bool {
	return len(s) >= 19 && s[4] == '-' && s[7] == '-' && s[10] == 'T' && s[13] == ':' && s[16] == ':'
}

type LineWriter struct {
	store   *Store
	channel string
	tee     io.Writer
	mu      sync.Mutex
	buf     bytes.Buffer
}

func NewLineWriter(store *Store, channel string, tee io.Writer) *LineWriter {
	return &LineWriter{store: store, channel: channel, tee: tee}
}

func (w *LineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.buf.Write(p)
	for {
		line, ok := w.nextLine()
		if !ok {
			break
		}
		if err := w.writeLine(line); err != nil {
			return len(p), err
		}
	}
	const truncatedSuffix = " [truncated]"
	chunkBytes := maxLineBytes - len(truncatedSuffix)
	for w.buf.Len() >= maxLineBytes {
		// Cut on a UTF-8 rune boundary at or below chunkBytes so a split multibyte rune is
		// not flushed as invalid bytes.
		b := w.buf.Bytes()
		cut := chunkBytes
		for cut > 0 && !utf8.RuneStart(b[cut]) {
			cut--
		}
		if cut == 0 {
			cut = chunkBytes
		}
		line := string(w.buf.Next(cut)) + truncatedSuffix
		if err := w.writeLine(line); err != nil {
			return len(p), err
		}
	}
	return len(p), nil
}

func (w *LineWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.buf.Len() == 0 {
		return nil
	}
	line := w.buf.String()
	w.buf.Reset()
	return w.writeLine(line)
}

func (w *LineWriter) writeLine(line string) error {
	if err := w.store.Append(w.channel, line); err != nil {
		return err
	}
	if w.tee != nil {
		_, _ = io.WriteString(w.tee, Redact(truncateBytes(line, maxLineBytes))+"\n")
	}
	return nil
}

func (w *LineWriter) nextLine() (string, bool) {
	b := w.buf.Bytes()
	i := bytes.IndexByte(b, '\n')
	if i < 0 {
		return "", false
	}
	line := string(b[:i])
	w.buf.Next(i + 1)
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return line, true
}

func Redact(s string) string {
	return redactString(s)
}

var redactPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)Bearer\s+[^\s,;]+`),
	regexp.MustCompile(`(?i)\b(Cookie|Set-Cookie):\s*[^\n]+`),
	regexp.MustCompile(`(?i)(--?[A-Za-z0-9_-]*(?:token|secret|key)[A-Za-z0-9_-]*)(=|\s+)("[^"]*"|[^\s,;&]+)`),
	regexp.MustCompile(`(?i)\b("?(?:[A-Za-z0-9_]*(?:token|secret|key)[A-Za-z0-9_]*|password|passwd|pwd|remixsid|remixdsid|vk_sid)"?\s*[:=]\s*)("[^"]*"|[^\s,;&]+)`),
}

func redactString(s string) string {
	for _, pattern := range redactPatterns {
		s = pattern.ReplaceAllStringFunc(s, redactMatch)
	}
	return s
}

func redactMatch(s string) string {
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "bearer") {
		return "Bearer [REDACTED]"
	}
	if strings.HasPrefix(lower, "cookie:") || strings.HasPrefix(lower, "set-cookie:") {
		return s[:strings.IndexByte(s, ':')+1] + " [REDACTED]"
	}
	if strings.HasPrefix(s, "-") {
		if i := strings.IndexByte(s, '='); i >= 0 {
			return s[:i+1] + "[REDACTED]"
		}
		fields := strings.Fields(s)
		if len(fields) > 0 {
			return fields[0] + " [REDACTED]"
		}
	}
	if i := strings.IndexAny(s, ":="); i >= 0 {
		return s[:i+1] + "[REDACTED]"
	}
	return "[REDACTED]"
}
