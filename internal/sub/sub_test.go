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

func TestMultiLinks(t *testing.T) {
	u := models.User{UUID: "11111111-1111-1111-1111-111111111111", Email: "a@b.c"}
	eps := []Endpoint{
		{Name: "本机", Host: "1.1.1.1", Port: 443, PublicKey: "p", ShortID: "s", ServerName: "www.microsoft.com", Flow: "xtls-rprx-vision"},
		{Name: "香港", Host: "2.2.2.2", Port: 443, PublicKey: "p", ShortID: "s", ServerName: "www.microsoft.com", Flow: "xtls-rprx-vision"},
	}
	links := LinksForEndpoints(eps, u)
	if len(links) != 2 {
		t.Fatal(links)
	}
	b64 := Base64Links(links)
	if b64 == "" {
		t.Fatal("empty b64")
	}
	y := ClashYAMLMulti(eps, u)
	if !strings.Contains(y, "1.1.1.1") || !strings.Contains(y, "2.2.2.2") || !strings.Contains(y, "PROXY") {
		t.Fatalf("clash: %s", y)
	}
}
