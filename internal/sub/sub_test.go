package sub

import (
	"strings"
	"testing"

	"hallo/internal/models"
)

func TestVLESSLink(t *testing.T) {
	in := models.Inbound{
		Port:       18443,
		Flow:       "xtls-rprx-vision",
		Dest:       "www.microsoft.com:443",
		ServerName: "www.microsoft.com",
		PublicKey:  "pub",
		ShortID:    "abcd",
	}
	u := models.User{UUID: "11111111-1111-1111-1111-111111111111", Email: "a@b.c"}
	link := VLESSLink("127.0.0.1", in, u)
	for _, want := range []string{"vless://", "127.0.0.1:18443", "security=reality", "pbk=pub", "sid=abcd"} {
		if !strings.Contains(link, want) {
			t.Fatalf("link %q missing %q", link, want)
		}
	}
}

func TestPublicHost(t *testing.T) {
	if g := PublicHost("http://1.2.3.4:18080", "x"); g != "1.2.3.4" {
		t.Fatalf("got %s", g)
	}
}
