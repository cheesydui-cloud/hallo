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
}
