package nodeconfig

import (
	"path/filepath"
	"testing"

	"hallo/internal/db"
	"hallo/internal/models"
)

func TestEndpointsPerNode(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })

	localID, err := d.CreateNode(models.Node{Name: "本机", Token: "t1", PublicHost: "1.1.1.1", Port: 443, IsLocal: true, Enabled: true, Subscribe: true})
	if err != nil {
		t.Fatal(err)
	}
	hkID, err := d.CreateNode(models.Node{Name: "香港", Token: "t2", PublicHost: "2.2.2.2", Port: 8443, Enabled: true, Subscribe: true})
	if err != nil {
		t.Fatal(err)
	}
	inLocal := models.Inbound{
		NodeID: localID, Port: 443, Flow: "xtls-rprx-vision", Dest: "www.microsoft.com:443",
		ServerName: "www.microsoft.com", PublicKey: "pub", ShortID: "abcd",
		PrivateKey: "priv", Protocol: "vless", Listen: "0.0.0.0", Enabled: true, Tag: "in-local",
	}
	if err := d.SaveInbound(&inLocal); err != nil {
		t.Fatal(err)
	}
	inHK := models.Inbound{
		NodeID: hkID, Port: 8443, Flow: "xtls-rprx-vision", Dest: "www.microsoft.com:443",
		ServerName: "www.microsoft.com", PublicKey: "pub2", ShortID: "ef01",
		PrivateKey: "priv2", Protocol: "vless", Listen: "0.0.0.0", Enabled: true, Tag: "in-hk",
	}
	if err := d.SaveInbound(&inHK); err != nil {
		t.Fatal(err)
	}
	u := models.User{Email: "a@b.c", UUID: "uuid-1", SubToken: "sub", Enabled: true}
	uid, err := d.CreateUser(u)
	if err != nil {
		t.Fatal(err)
	}
	u.ID = uid

	eps, err := Endpoints(d, u, inLocal, "9.9.9.9")
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
	eps, err = Endpoints(d, u, inLocal, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 1 || eps[0].Host != "2.2.2.2" || eps[0].Port != 8443 {
		t.Fatalf("restricted endpoints %#v", eps)
	}

	hk, _ := d.GetNode(hkID)
	cfg, err := Build(d, *hk, inLocal)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(cfg["inbounds"].([]any)); got != 1 {
		t.Fatalf("hk node should only have its own inbound, got %d", got)
	}
	inb := cfg["inbounds"].([]any)[0].(map[string]any)
	if inb["port"] != 8443 && inb["port"] != float64(8443) {
		t.Fatalf("hk inbound port %#v", inb["port"])
	}
	clients := inb["settings"].(map[string]any)["clients"].([]map[string]any)
	if len(clients) != 1 || clients[0]["id"] != "uuid-1" {
		t.Fatalf("clients %#v", clients)
	}

	local, _ := d.GetNode(localID)
	lcfg, err := Build(d, *local, inLocal)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(lcfg["inbounds"].([]any)); got != 1 {
		t.Fatalf("local node should only have its own inbound, got %d", got)
	}
}
