package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"hallo/internal/config"
	"hallo/internal/db"
	"hallo/internal/xray"
)

func TestSetupInboundAndSub(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "hallo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	xm := xray.New(filepath.Join(dir, "missing-xray"), filepath.Join(dir, "xray", "config.json"))
	s := New(config.Config{Listen: ":0", DataDir: dir}, database, xm, nil, "test")
	ts := httptest.NewServer(s.Router())
	t.Cleanup(ts.Close)

	body := map[string]any{"username": "admin", "password": "secret1", "public_url": ts.URL, "port": 443}
	raw, _ := json.Marshal(body)
	res, err := http.Post(ts.URL+"/api/setup", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("setup %d %s", res.StatusCode, b)
	}

	inReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/inbound", nil)
	for _, c := range res.Cookies() {
		inReq.AddCookie(c)
	}
	inRes, err := http.DefaultClient.Do(inReq)
	if err != nil {
		t.Fatal(err)
	}
	defer inRes.Body.Close()
	if inRes.StatusCode != 200 {
		b, _ := io.ReadAll(inRes.Body)
		t.Fatalf("inbound %s %s", inRes.Status, b)
	}
	var in map[string]any
	if err := json.NewDecoder(inRes.Body).Decode(&in); err != nil {
		t.Fatal(err)
	}
	if in["keys_ok"] != true {
		t.Fatalf("keys not ok: %#v", in)
	}
	if in["private_key"] == "CHANGE_ME_PRIVATE" {
		t.Fatal("placeholder leaked")
	}

	uRaw, _ := json.Marshal(map[string]any{"email": "alice", "remark": "me"})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/users", bytes.NewReader(uRaw))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range res.Cookies() {
		req.AddCookie(c)
	}
	ures, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer ures.Body.Close()
	if ures.StatusCode != 200 {
		b, _ := io.ReadAll(ures.Body)
		t.Fatalf("user %d %s", ures.StatusCode, b)
	}

	listReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/users", nil)
	for _, c := range res.Cookies() {
		listReq.AddCookie(c)
	}
	lres, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatal(err)
	}
	defer lres.Body.Close()
	var list struct {
		Items []struct {
			SubURL     string   `json:"sub_url"`
			VlessLinks []string `json:"vless_links"`
		} `json:"items"`
	}
	if err := json.NewDecoder(lres.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].SubURL == "" {
		t.Fatalf("%#v", list)
	}
	sres, err := http.Get(list.Items[0].SubURL)
	if err != nil {
		t.Fatal(err)
	}
	defer sres.Body.Close()
	if sres.StatusCode != 200 {
		t.Fatal(sres.Status)
	}
	b, _ := io.ReadAll(sres.Body)
	if len(b) < 8 {
		t.Fatalf("sub too short %q", b)
	}

	nreq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/nodes", nil)
	for _, c := range res.Cookies() {
		nreq.AddCookie(c)
	}
	nres, err := http.DefaultClient.Do(nreq)
	if err != nil {
		t.Fatal(err)
	}
	defer nres.Body.Close()
	if nres.StatusCode != 200 {
		b, _ := io.ReadAll(nres.Body)
		t.Fatalf("nodes %s %s", nres.Status, b)
	}
	var nodes struct {
		Items []struct {
			Name    string `json:"name"`
			IsLocal bool   `json:"is_local"`
		} `json:"items"`
	}
	if err := json.NewDecoder(nres.Body).Decode(&nodes); err != nil {
		t.Fatal(err)
	}
	if len(nodes.Items) == 0 {
		t.Fatal("expected local node")
	}
}

func TestRegenKeysNoXray(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "hallo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	xm := xray.New("", filepath.Join(dir, "xray", "config.json"))
	s := New(config.Config{DataDir: dir}, database, xm, nil, "test")
	ts := httptest.NewServer(s.Router())
	t.Cleanup(ts.Close)
	raw, _ := json.Marshal(map[string]any{"username": "admin", "password": "secret1", "port": 18443})
	res, err := http.Post(ts.URL+"/api/setup", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	cookies := res.Cookies()
	res.Body.Close()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/inbound/regen-keys", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	r2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Body.Close()
	if r2.StatusCode != 200 {
		b, _ := io.ReadAll(r2.Body)
		t.Fatalf("%d %s", r2.StatusCode, b)
	}
	var in map[string]any
	_ = json.NewDecoder(r2.Body).Decode(&in)
	if in["keys_ok"] != true {
		t.Fatalf("%#v", in)
	}
}

func cookieReq(method, url string, body []byte, cookies []*http.Cookie) *http.Request {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, _ := http.NewRequest(method, url, rdr)
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	return req
}

func TestInboundsOutboundsAndNode(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "hallo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	xm := xray.New(filepath.Join(dir, "missing-xray"), filepath.Join(dir, "xray", "config.json"))
	s := New(config.Config{Listen: ":0", DataDir: dir}, database, xm, nil, "test")
	ts := httptest.NewServer(s.Router())
	t.Cleanup(ts.Close)

	raw, _ := json.Marshal(map[string]any{"username": "admin", "password": "secret1", "public_url": ts.URL, "port": 443})
	res, err := http.Post(ts.URL+"/api/setup", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	cookies := res.Cookies()
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("setup %d", res.StatusCode)
	}

	listRes, err := http.DefaultClient.Do(cookieReq(http.MethodGet, ts.URL+"/api/inbounds", nil, cookies))
	if err != nil {
		t.Fatal(err)
	}
	defer listRes.Body.Close()
	if listRes.StatusCode != 200 {
		b, _ := io.ReadAll(listRes.Body)
		t.Fatalf("inbounds %s %s", listRes.Status, b)
	}
	var listed struct {
		Items []struct {
			ID     int64 `json:"id"`
			NodeID int64 `json:"node_id"`
			Port   int   `json:"port"`
			KeysOK bool  `json:"keys_ok"`
		} `json:"items"`
	}
	if err := json.NewDecoder(listRes.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) == 0 || !listed.Items[0].KeysOK {
		t.Fatalf("inbounds %#v", listed.Items)
	}

	nres, err := http.DefaultClient.Do(cookieReq(http.MethodGet, ts.URL+"/api/nodes", nil, cookies))
	if err != nil {
		t.Fatal(err)
	}
	var nodes struct {
		Items []struct {
			ID      int64  `json:"id"`
			IsLocal bool   `json:"is_local"`
			Name    string `json:"name"`
		} `json:"items"`
	}
	_ = json.NewDecoder(nres.Body).Decode(&nodes)
	nres.Body.Close()
	if len(nodes.Items) == 0 || !nodes.Items[0].IsLocal {
		t.Fatalf("nodes %#v", nodes.Items)
	}

	badIn, _ := json.Marshal(map[string]any{"remark": "no-node", "port": 443})
	badRes, err := http.DefaultClient.Do(cookieReq(http.MethodPost, ts.URL+"/api/inbounds", badIn, cookies))
	if err != nil {
		t.Fatal(err)
	}
	if badRes.StatusCode == 200 {
		t.Fatal("inbound without node_id should fail")
	}
	badRes.Body.Close()

	createBody, _ := json.Marshal(map[string]any{
		"name": "洛杉矶", "public_host": "8.8.8.8", "port": 443,
	})
	cres, err := http.DefaultClient.Do(cookieReq(http.MethodPost, ts.URL+"/api/nodes", createBody, cookies))
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		Node struct {
			ID int64 `json:"id"`
		} `json:"node"`
	}
	_ = json.NewDecoder(cres.Body).Decode(&created)
	cres.Body.Close()
	if created.Node.ID == 0 {
		t.Fatal("node not created")
	}

	listRes2, err := http.DefaultClient.Do(cookieReq(http.MethodGet, ts.URL+"/api/inbounds", nil, cookies))
	if err != nil {
		t.Fatal(err)
	}
	var listed2 struct {
		Items []struct {
			NodeID int64 `json:"node_id"`
		} `json:"items"`
	}
	_ = json.NewDecoder(listRes2.Body).Decode(&listed2)
	listRes2.Body.Close()
	foundNodeIn := false
	for _, it := range listed2.Items {
		if it.NodeID == created.Node.ID {
			foundNodeIn = true
		}
	}
	if !foundNodeIn {
		t.Fatalf("new node missing inbound %#v", listed2.Items)
	}

	ores, err := http.DefaultClient.Do(cookieReq(http.MethodGet, ts.URL+"/api/outbounds", nil, cookies))
	if err != nil {
		t.Fatal(err)
	}
	var outs struct {
		Items []struct {
			Tag       string `json:"tag"`
			IsDefault bool   `json:"is_default"`
		} `json:"items"`
	}
	_ = json.NewDecoder(ores.Body).Decode(&outs)
	ores.Body.Close()
	foundDirect := false
	for _, o := range outs.Items {
		if o.Tag == "direct" && o.IsDefault {
			foundDirect = true
		}
	}
	if !foundDirect {
		t.Fatalf("missing default direct outbound %#v", outs.Items)
	}

	vlessOut, _ := json.Marshal(map[string]any{
		"remark": "链式", "tag": "chain", "protocol": "vless",
		"address": "1.1.1.1", "port": 443, "uuid": "11111111-1111-1111-1111-111111111111",
		"public_key": "pk", "server_name": "www.microsoft.com",
	})
	ores2, err := http.DefaultClient.Do(cookieReq(http.MethodPost, ts.URL+"/api/outbounds", vlessOut, cookies))
	if err != nil {
		t.Fatal(err)
	}
	if ores2.StatusCode != 200 {
		bb, _ := io.ReadAll(ores2.Body)
		t.Fatalf("create outbound %s %s", ores2.Status, bb)
	}
	ores2.Body.Close()
}
