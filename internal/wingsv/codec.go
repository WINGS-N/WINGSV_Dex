// Package wingsv encodes and decodes wingsv:// links. The wire format matches the
// Android client (WingsImportParser): a wingsv:// link is base64url of a single
// format byte followed by the zlib-deflated protobuf Config.
package wingsv

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/WINGS-N/wingsv-dex/internal/gen/wingsvpb"
)

const (
	SchemePrefix = "wingsv://"
	// formatProtobufDeflate is the single leading payload byte tagging the rest as
	// zlib-deflated protobuf. It is the only format emitted.
	formatProtobufDeflate = 0x12
)

// linkPattern matches a wingsv:// link inside arbitrary pasted text, mirroring the
// app's LINK_PATTERN. The body allows both url-safe and standard base64 alphabets.
var (
	linkPattern     = regexp.MustCompile(`wingsv://[A-Za-z0-9_\-+/=]+`)
	whitespaceRegex = regexp.MustCompile(`\s+`)
)

// ExtractLink pulls a wingsv:// link out of arbitrary text: a
// regex scan first, then the whole trimmed string if it already is a link.
func ExtractLink(text string) string {
	text = strings.TrimSpace(text)
	if m := linkPattern.FindString(text); m != "" {
		return m
	}
	if strings.HasPrefix(text, SchemePrefix) {
		return text
	}
	return ""
}

// Decode parses a wingsv:// link (or text containing one) into its Config. It is
// lenient: it extracts the link from surrounding
// text, strips stray slashes and whitespace, fixes base64 padding, and accepts both
// the url-safe and standard base64 alphabets.
func Decode(raw string) (*wingsvpb.Config, error) {
	link := ExtractLink(raw)
	if link == "" {
		return nil, errors.New("no wingsv:// link found")
	}
	decoded, err := decodePayload(link)
	if err != nil {
		return nil, err
	}
	if len(decoded) == 0 {
		return nil, errors.New("wingsv: empty payload")
	}
	if decoded[0] != formatProtobufDeflate {
		return nil, fmt.Errorf("wingsv: unsupported payload format 0x%02x", decoded[0])
	}
	inflated, err := inflate(decoded[1:])
	if err != nil {
		return nil, err
	}
	config := &wingsvpb.Config{}
	if err := proto.Unmarshal(inflated, config); err != nil {
		return nil, fmt.Errorf("wingsv: protobuf: %w", err)
	}
	return config, nil
}

// decodePayload turns the base64 body of a link into bytes's
// decodePayload: trim, drop leading slashes, strip whitespace, repad, then try the
// url-safe alphabet and fall back to standard base64.
func decodePayload(link string) ([]byte, error) {
	payload := strings.TrimSpace(link[len(SchemePrefix):])
	payload = strings.TrimLeft(payload, "/")
	payload = whitespaceRegex.ReplaceAllString(payload, "")
	payload = normalizePadding(payload)
	if b, err := base64.URLEncoding.DecodeString(payload); err == nil {
		return b, nil
	}
	b, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("wingsv: base64: %w", err)
	}
	return b, nil
}

// normalizePadding pads the payload out to a multiple of four with '=', matching
// the app so both padded and stripped links decode.
func normalizePadding(payload string) string {
	if mod := len(payload) % 4; mod != 0 {
		payload += strings.Repeat("=", 4-mod)
	}
	return payload
}

// Encode serializes a Config into a wingsv:// link: protobuf bytes, zlib deflate at
// best compression, a leading format byte, then base64url.
func Encode(config *wingsvpb.Config) (string, error) {
	raw, err := proto.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("wingsv: protobuf: %w", err)
	}
	var buf bytes.Buffer
	buf.WriteByte(formatProtobufDeflate)
	zw, err := zlib.NewWriterLevel(&buf, flate.BestCompression)
	if err != nil {
		return "", fmt.Errorf("wingsv: zlib: %w", err)
	}
	if _, err := zw.Write(raw); err != nil {
		return "", fmt.Errorf("wingsv: zlib write: %w", err)
	}
	if err := zw.Close(); err != nil {
		return "", fmt.Errorf("wingsv: zlib close: %w", err)
	}
	return SchemePrefix + base64.URLEncoding.EncodeToString(buf.Bytes()), nil
}

// inflate decompresses the payload. The app writes a zlib stream; fall back to a
// raw deflate stream so a truncated or alternately framed link still decodes.
func inflate(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, errors.New("wingsv: empty compressed payload")
	}
	if zr, err := zlib.NewReader(bytes.NewReader(payload)); err == nil {
		defer zr.Close()
		return io.ReadAll(zr)
	}
	fr := flate.NewReader(bytes.NewReader(payload))
	defer fr.Close()
	out, err := io.ReadAll(fr)
	if err != nil {
		return nil, fmt.Errorf("wingsv: link is corrupt or truncated: %w", err)
	}
	return out, nil
}
