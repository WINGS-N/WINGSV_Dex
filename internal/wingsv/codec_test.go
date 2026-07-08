package wingsv

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/WINGS-N/wingsv-dex/internal/gen/wingsvpb"
)

func sampleConfig() *wingsvpb.Config {
	return &wingsvpb.Config{
		Ver:     1,
		Type:    wingsvpb.ConfigType_CONFIG_TYPE_VK_TURN_PROFILE,
		Backend: wingsvpb.BackendType_BACKEND_TYPE_VK_TURN,
		Turn: &wingsvpb.Turn{
			Endpoint:    &wingsvpb.Endpoint{Host: "203.0.113.10", Port: 51820},
			Threads:     proto.Uint32(24),
			UseUdp:      proto.Bool(true),
			SessionMode: wingsvpb.TurnSessionMode_TURN_SESSION_MODE_MUX,
			TunnelMode:  wingsvpb.TunnelMode_TUNNEL_MODE_WIREGUARD,
			Title:       "VK TURN",
		},
		Wg: &wingsvpb.WireGuard{
			Iface: &wingsvpb.Interface{Addrs: []string{"10.8.0.2/32"}},
			Peer: &wingsvpb.Peer{
				AllowedIps: []*wingsvpb.Cidr{{Addr: []byte{0, 0, 0, 0}, Prefix: 0}},
			},
			Endpoint: &wingsvpb.Endpoint{Host: "203.0.113.10", Port: 51820},
		},
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	link, err := Encode(sampleConfig())
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !strings.HasPrefix(link, SchemePrefix) {
		t.Fatalf("link missing scheme prefix: %q", link)
	}
	got, err := Decode(link)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !proto.Equal(got, sampleConfig()) {
		t.Fatalf("round-trip mismatch:\n got:  %v\n want: %v", got, sampleConfig())
	}
}

func TestDecodeRejectsNonWings(t *testing.T) {
	if _, err := Decode("vless://whatever"); err == nil {
		t.Fatal("expected error for non-wingsv link")
	}
}

func TestDecodeRejectsBadFormatByte(t *testing.T) {
	// A valid base64url payload whose first byte is not the protobuf-deflate tag.
	if _, err := Decode("wingsv://AAAA"); err == nil {
		t.Fatal("expected error for unknown format byte")
	}
}

// TestDecodeFromSurroundingText lenient parser: a link embedded
// in other text, with whitespace, must still decode.
func TestDecodeFromSurroundingText(t *testing.T) {
	link, err := Encode(sampleConfig())
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	wrapped := "Мой конфиг:\n  " + link + "  \nспасибо"
	got, err := Decode(wrapped)
	if err != nil {
		t.Fatalf("Decode from text: %v", err)
	}
	if !proto.Equal(got, sampleConfig()) {
		t.Fatal("decoded config from surrounding text does not match")
	}
}

func TestExtractLink(t *testing.T) {
	if got := ExtractLink("prefix wingsv://ABC-_123 suffix"); got != "wingsv://ABC-_123" {
		t.Fatalf("ExtractLink = %q", got)
	}
	if got := ExtractLink("no link here"); got != "" {
		t.Fatalf("ExtractLink should be empty, got %q", got)
	}
}
