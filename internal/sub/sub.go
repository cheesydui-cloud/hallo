package sub

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"hallo/internal/models"
)

func PublicHost(publicURL, fallback string) string {
	if publicURL != "" {
		u, err := url.Parse(publicURL)
		if err == nil && u.Hostname() != "" {
			return u.Hostname()
		}
		s := strings.TrimPrefix(publicURL, "http://")
		s = strings.TrimPrefix(s, "https://")
		s = strings.Split(s, "/")[0]
		host, _, err := net.SplitHostPort(s)
		if err == nil {
			return host
		}
		return s
	}
	return fallback
}

func VLESSLink(host string, in models.Inbound, u models.User) string {
	if host == "" {
		host = "127.0.0.1"
	}
	sni := in.ServerName
	if sni == "" {
		sni = strings.Split(in.Dest, ":")[0]
	}
	q := url.Values{}
	q.Set("encryption", "none")
	q.Set("security", "reality")
	q.Set("type", "tcp")
	q.Set("sni", sni)
	q.Set("fp", "chrome")
	q.Set("pbk", in.PublicKey)
	q.Set("sid", in.ShortID)
	if in.Flow != "" {
		q.Set("flow", in.Flow)
	}
	name := u.Email
	if u.Remark != "" {
		name = u.Remark
	}
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s",
		u.UUID, host, in.Port, q.Encode(), url.QueryEscape(name))
}

func ClashYAML(host string, in models.Inbound, u models.User) string {
	if host == "" {
		host = "127.0.0.1"
	}
	sni := in.ServerName
	if sni == "" {
		sni = strings.Split(in.Dest, ":")[0]
	}
	name := u.Email
	if u.Remark != "" {
		name = u.Remark
	}
	var b strings.Builder
	b.WriteString("mixed-port: 7890\n")
	b.WriteString("allow-lan: false\n")
	b.WriteString("mode: rule\n")
	b.WriteString("proxies:\n")
	b.WriteString("  - name: " + strconv.Quote(name) + "\n")
	b.WriteString("    type: vless\n")
	b.WriteString("    server: " + host + "\n")
	b.WriteString("    port: " + strconv.Itoa(in.Port) + "\n")
	b.WriteString("    uuid: " + u.UUID + "\n")
	b.WriteString("    network: tcp\n")
	b.WriteString("    tls: true\n")
	b.WriteString("    udp: true\n")
	b.WriteString("    flow: " + in.Flow + "\n")
	b.WriteString("    servername: " + sni + "\n")
	b.WriteString("    client-fingerprint: chrome\n")
	b.WriteString("    reality-opts:\n")
	b.WriteString("      public-key: " + in.PublicKey + "\n")
	b.WriteString("      short-id: " + in.ShortID + "\n")
	b.WriteString("proxy-groups:\n")
	b.WriteString("  - name: PROXY\n")
	b.WriteString("    type: select\n")
	b.WriteString("    proxies:\n")
	b.WriteString("      - " + strconv.Quote(name) + "\n")
	b.WriteString("rules:\n")
	b.WriteString("  - MATCH,PROXY\n")
	return b.String()
}

func Base64VLESS(host string, in models.Inbound, u models.User) string {
	link := VLESSLink(host, in, u)
	return base64.StdEncoding.EncodeToString([]byte(link + "\n"))
}

func SubURL(publicURL, token string) string {
	base := strings.TrimRight(publicURL, "/")
	if base == "" {
		base = ""
	}
	return base + "/sub/" + token
}
