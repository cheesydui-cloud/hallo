package nodeconfig

import (
	"path/filepath"
	"testing"

	"hallo/internal/db"
	"hallo/internal/models"
	"hallo/internal/xray"
)

func TestEndpointsAndRelay(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })

	in := models.Inbound{
		Port: 443, Flow: "xtls-rprx-vision", Dest: "www.microsoft.com:443",
		ServerName: "www.microsoft.com", PublicKey: "pub", ShortID: "abcd",
		PrivateKey: "priv", Protocol: "vless", Listen: "0.0.0.0",
	}
	if err := d.SaveInbound(&in); err != nil {
		t.Fatal(err)
	}
	localID, err := d.CreateNode(models.Node{Name: "本机", Token: "t1", PublicHost: "1.1.1.1", Port: 443, IsLocal: true, Enabled: true, Subscribe: true})
	if err != nil {
		t.Fatal(err)
	}
	relayTo := localID
	hkID, err := d.CreateNode(models.Node{Name: "香港", Token: "t2", PublicHost: "2.2.2.2", Port: 443, Enabled: true, Subscribe: true, RelayNodeID: &relayTo, RelayUUID: "11111111-1111-1111-1111-111111111111"})
	if err != nil {
		t.Fatal(err)
	}
	u := models.User{Email: "a@b.c", UUID: "uuid-1", SubToken: "sub", Enabled: true}
	uid, err := d.CreateUser(u)
	if err != nil {
		t.Fatal(err)
	}
	u.ID = uid

	eps, err := Endpoints(d, u, in, "9.9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 2 {
		t.Fatalf("want 2 endpoints, got %#v", eps)
	}

	if err := d.SetUserNodes(uid, []int64{hkID}); err != nil {
		t.Fatal(err)
	}
	u.NodeIDs = []int64{hkID}
	eps, err = Endpoints(d, u, in, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 1 || eps[0].Host != "2.2.2.2" {
		t.Fatalf("restricted endpoints %#v", eps)
	}

	hk, _ := d.GetNode(hkID)
	cfg, err := Build(d, *hk, in)
	if err != nil {
		t.Fatal(err)
	}
	outs := cfg["outbounds"].([]any)
	first := outs[0].(map[string]any)
	if first["tag"] != "relay" {
		t.Fatalf("expected relay outbound, got %#v", first)
	}
	local, _ := d.GetNode(localID)
	lcfg, err := Build(d, *local, in)
	if err != nil {
		t.Fatal(err)
	}
	inb := lcfg["inbounds"].([]any)[0].(map[string]any)
	clients := inb["settings"].(map[string]any)["clients"].([]map[string]any)
	found := false
	for _, c := range clients {
		if c["id"] == "11111111-1111-1111-1111-111111111111" {
			found = true
		}
	}
	if !found {
		t.Fatalf("relay uuid missing on target: %#v", clients)
	}
	_ = xray.ValidRealityKey
}
