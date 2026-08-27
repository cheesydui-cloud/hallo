package xray

import (
	"encoding/base64"
	"testing"

	"hallo/internal/models"
)

func TestParseLimitBytes(t *testing.T) {
	cases := map[string]int64{
		"0":    0,
		"10g":  10 << 30,
		"100m": 100 << 20,
		"12":   12,
	}
	for in, want := range cases {
		got, err := ParseLimitBytes(in)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s: got %d want %d", in, got, want)
		}
	}
}

func TestGenerateRealityKeysNative(t *testing.T) {
	priv, pub, err := GenerateRealityKeys("")
	if err != nil {
		t.Fatal(err)
	}
	if !ValidRealityKey(priv) || !ValidRealityKey(pub) {
		t.Fatalf("invalid keys priv=%q pub=%q", priv, pub)
	}
	if priv == pub {
		t.Fatal("private and public should differ")
	}
	raw, err := base64.RawURLEncoding.DecodeString(priv)
	if err != nil || len(raw) != 32 {
		t.Fatalf("private decode: %v len=%d", err, len(raw))
	}
}

func TestIsPlaceholderKey(t *testing.T) {
	if !IsPlaceholderKey("CHANGE_ME_PRIVATE") || !IsPlaceholderKey("") {
		t.Fatal("expected placeholders")
	}
	if ValidRealityKey("CHANGE_ME_PUBLIC") {
		t.Fatal("placeholder must not be valid")
	}
}

func TestBuildConfigRelay(t *testing.T) {
	in := models.Inbound{Port: 443, Flow: "xtls-rprx-vision", Dest: "www.microsoft.com:443", ServerName: "www.microsoft.com", PrivateKey: "k", PublicKey: "p", ShortID: "s"}
	users := []models.User{{UUID: "u", Email: "e"}}
	cfg := BuildConfig(in, users, &Relay{Address: "8.8.8.8", Port: 443, UUID: "relay-u", PublicKey: "p", ShortID: "s", ServerName: "www.microsoft.com"})
	outs := cfg["outbounds"].([]any)
	if outs[0].(map[string]any)["tag"] != "relay" {
		t.Fatalf("%#v", outs[0])
	}
	rules := cfg["routing"].(map[string]any)["rules"].([]any)
	if rules[0].(map[string]any)["outboundTag"] != "relay" {
		t.Fatalf("%#v", rules)
	}
}

func TestBuildFullCustomOutbound(t *testing.T) {
	in := models.Inbound{Port: 443, Protocol: "vless", Listen: "0.0.0.0", Dest: "www.microsoft.com:443", ServerName: "www.microsoft.com", PrivateKey: "k", PublicKey: "p", ShortID: "s", Enabled: true, Tag: "vless-in"}
	users := []models.User{{UUID: "u", Email: "e"}}
	outs := []models.Outbound{
		{Tag: "direct", Protocol: "freedom", Enabled: true, IsDefault: false},
		{Tag: "warp", Protocol: "socks", Address: "127.0.0.1", Port: 40000, Enabled: true, IsDefault: true},
	}
	cfg := BuildFull([]models.Inbound{in}, users, outs, nil)
	rules := cfg["routing"].(map[string]any)["rules"].([]any)
	if rules[0].(map[string]any)["outboundTag"] != "warp" {
		t.Fatalf("default outbound want warp, got %#v", rules)
	}
	inb := cfg["inbounds"].([]any)[0].(map[string]any)
	sniff := inb["sniffing"].(map[string]any)
	if sniff["routeOnly"] != true {
		t.Fatalf("Vision sniffing must be routeOnly, got %#v", sniff)
	}
}

func TestBuildFullVMessAndShadowsocks(t *testing.T) {
	users := []models.User{{UUID: "u", Email: "e"}}
	ins := []models.Inbound{
		{Tag: "vless-in", Protocol: "vless", Port: 443, Security: "reality", Dest: "www.microsoft.com:443", ServerName: "www.microsoft.com", PrivateKey: "k", PublicKey: "p", ShortID: "s", Enabled: true},
		{Tag: "vmess-in", Protocol: "vmess", Port: 2053, Listen: "0.0.0.0", Enabled: true},
		{Tag: "ss-in", Protocol: "shadowsocks", Port: 8388, Method: "aes-128-gcm", Password: "secret", Enabled: true},
	}
	cfg := BuildFull(ins, users, nil, nil)
	got := cfg["inbounds"].([]any)
	if len(got) != 3 {
		t.Fatalf("want 3 inbounds, got %d", len(got))
	}
	vmess := got[1].(map[string]any)
	if vmess["protocol"] != "vmess" || vmess["port"] != 2053 {
		t.Fatalf("vmess %#v", vmess)
	}
	ss := got[2].(map[string]any)
	if ss["protocol"] != "shadowsocks" {
		t.Fatalf("ss %#v", ss)
	}
	settings := ss["settings"].(map[string]any)
	if settings["password"] != "secret" || settings["method"] != "aes-128-gcm" {
		t.Fatalf("ss settings %#v", settings)
	}
}

func TestNormalizeSS2022(t *testing.T) {
	method, pw, err := NormalizeSS("2022-blake3-aes-128-gcm", "not-valid")
	if err != nil {
		t.Fatal(err)
	}
	if method != "2022-blake3-aes-128-gcm" {
		t.Fatalf("method %s", method)
	}
	if !ValidSS2022Password(method, pw) {
		t.Fatalf("generated 2022 password invalid: %q", pw)
	}
	m2, p2, err := NormalizeSS(method, pw)
	if err != nil || m2 != method || p2 != pw {
		t.Fatalf("should keep valid password, got %s %s", m2, p2)
	}
}
