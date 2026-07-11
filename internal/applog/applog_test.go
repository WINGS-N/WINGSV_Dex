package applog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNewStoreCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs", "nested")
	if _, err := NewStore(dir); err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("stat dir: %v", err)
	}
}

func TestAppendSnapshotOrderAndClear(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := s.Append("runtime", "first"); err != nil {
		t.Fatalf("append1: %v", err)
	}
	if err := s.Append("runtime", "second"); err != nil {
		t.Fatalf("append2: %v", err)
	}
	snap, err := s.Snapshot(ChannelRuntime)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snap.Lines) != 2 || !strings.Contains(snap.Lines[0], "first") || !strings.Contains(snap.Lines[1], "second") {
		t.Fatalf("unexpected lines: %#v", snap.Lines)
	}
	if snap.Version != 2 {
		t.Fatalf("version = %d, want 2", snap.Version)
	}
	if err := s.Clear("runtime"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	snap, err = s.Snapshot("runtime")
	if err != nil {
		t.Fatalf("snapshot after clear: %v", err)
	}
	if len(snap.Lines) != 0 || snap.Text != "" {
		t.Fatalf("clear did not empty log: %#v", snap)
	}
}

func TestSnapshotIsJSONTagged(t *testing.T) {
	snap := Snapshot{Channel: ChannelRuntime, Lines: []string{"a"}, Text: "a", Version: 1}
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	for _, want := range []string{"\"channel\"", "\"lines\"", "\"text\"", "\"version\""} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
}

func TestAppendBoundsFile(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	line := strings.Repeat("x", 4096)
	for i := 0; i < 80; i++ {
		if err := s.Append("proxy", line+"-"+strings.Repeat("y", 4096)); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	snap, err := s.Snapshot(ChannelProxy)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snap.Text) > MaxFileBytes {
		t.Fatalf("snapshot too large: %d", len(snap.Text))
	}
	if len(snap.Lines) == 0 {
		t.Fatal("expected retained lines")
	}
}

func TestAppendAfterBoundKeepsLineSeparation(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	line := strings.Repeat("x", 8192)
	for i := 0; i < 40; i++ {
		if err := s.Append(ChannelRuntime, line); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	if err := s.Append(ChannelRuntime, "tail"); err != nil {
		t.Fatalf("append tail: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(s.dir, "runtime.log"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.HasSuffix(string(raw), "\n") {
		t.Fatalf("missing trailing newline")
	}
}

func TestConcurrentWrites(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				if err := s.Append("runtime", strings.Repeat("a", i+1)); err != nil {
					t.Errorf("append: %v", err)
					return
				}
			}
		}(i)
	}
	wg.Wait()
	snap, err := s.Snapshot("runtime")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snap.Lines) != 500 {
		t.Fatalf("want 500 lines, got %d", len(snap.Lines))
	}
}

func TestLineWriterPartialAndMultiline(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	var tee strings.Builder
	w := NewLineWriter(s, "proxy", &tee)
	if _, err := w.Write([]byte("one\ntwo\naccessTo")); err != nil {
		t.Fatalf("write1: %v", err)
	}
	if _, err := w.Write([]byte("ken=abc\nthree")); err != nil {
		t.Fatalf("write2: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	snap, err := s.Snapshot("proxy")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snap.Lines) != 4 {
		t.Fatalf("want 4 lines, got %d: %#v", len(snap.Lines), snap.Lines)
	}
	if !strings.Contains(snap.Lines[2], "accessToken") || !strings.Contains(snap.Lines[3], "three") {
		t.Fatalf("unexpected contents: %#v tee=%q", snap.Lines, tee.String())
	}
	if strings.Contains(tee.String(), "abc") || !strings.Contains(tee.String(), "accessToken=[REDACTED]\n") || !strings.Contains(tee.String(), "three\n") {
		t.Fatalf("tee was not line-sanitized: %q", tee.String())
	}
}

func TestRedact(t *testing.T) {
	in := `Bearer abc def -app-grpc-token flag1 --app-grpc-token=flag2 wrap_key_hex: wrap wrapKeyHex=wrap2 remixsid=vkcookie presharedKey: x password=pass access_token="tok" authToken:tok2 refreshToken=refresh idToken=id apiKey=api clientSecret=client client_secret=client2 secret_key=sek vk_access_token=vk "access_token":"abc" "privateKey": "abc" token="quoted" Cookie: a=b; c=d Set-Cookie: sid=1; HttpOnly`
	out := Redact(in)
	for _, bad := range []string{"abc", "flag1", "flag2", ": wrap", "=wrap2", "vkcookie", "=\"tok\"", "tok2", "=refresh", "=api", "=client", "=client2", "=sek", "=vk", "quoted", "a=b", "sid=1"} {
		if strings.Contains(out, bad) {
			t.Fatalf("redaction failed for %q: %q", bad, out)
		}
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("expected redaction marker: %q", out)
	}
}

func TestLineWriterBoundsPartialLine(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	w := NewLineWriter(s, ChannelProxy, nil)
	if _, err := w.Write([]byte(strings.Repeat("x", maxLineBytes*3))); err != nil {
		t.Fatalf("write: %v", err)
	}
	if w.buf.Len() >= maxLineBytes {
		t.Fatalf("partial buffer not bounded: %d", w.buf.Len())
	}
	snap, err := s.Snapshot(ChannelProxy)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snap.Lines) == 0 {
		t.Fatalf("expected bounded log chunks: %#v", snap.Lines)
	}
}

func TestUnknownChannel(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := s.Snapshot("nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestFlushRedactsPartialLine(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	var tee strings.Builder
	w := NewLineWriter(s, ChannelProxy, &tee)
	if _, err := w.Write([]byte(`accessToken=abc`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if strings.Contains(tee.String(), "abc") {
		t.Fatalf("tee leaked secret: %q", tee.String())
	}
	if !strings.Contains(tee.String(), "[REDACTED]\n") {
		t.Fatalf("tee missing redacted line: %q", tee.String())
	}
}
