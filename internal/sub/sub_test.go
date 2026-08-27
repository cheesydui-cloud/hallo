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

func TestShareLinkVMessAndSS(t *testing.T) {
	u := models.User{UUID: "11111111-1111-1111-1111-111111111111", Email: "a@b.c"}
	vm := ShareLink(Endpoint{Protocol: "vmess", Host: "1.1.1.1", Port: 2053, Name: "洛杉矶-vmess"}, u, "")
	if !strings.HasPrefix(vm, "vmess://") {
		t.Fatalf("vmess link %q", vm)
	}
	ss := ShareLink(Endpoint{Protocol: "shadowsocks", Host: "2.2.2.2", Port: 8388, Method: "aes-128-gcm", Password: "pw", Name: "ss"}, u, "")
	if !strings.HasPrefix(ss, "ss://") || !strings.Contains(ss, "2.2.2.2:8388") {
		t.Fatalf("ss link %q", ss)
	}
	y := ClashYAMLMulti([]Endpoint{
		{Protocol: "vmess", Host: "1.1.1.1", Port: 2053, Name: "vm"},
		{Protocol: "shadowsocks", Host: "2.2.2.2", Port: 8388, Method: "aes-128-gcm", Password: "pw", Name: "ss"},
	}, u)
	if !strings.Contains(y, "type: vmess") || !strings.Contains(y, "type: ss") {
		t.Fatalf("clash multi: %s", y)
	}
}
