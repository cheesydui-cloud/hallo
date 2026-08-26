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

type Endpoint struct {
	Name       string
	Host       string
	Port       int
	Flow       string
	ServerName string
	PublicKey  string
	ShortID    string
}

func VLESSLink(host string, in models.Inbound, u models.User) string {
	return VLESSLinkNamed(host, in, u, "")
}

func VLESSLinkNamed(host string, in models.Inbound, u models.User, nodeName string) string {
	if host == "" {
		host = "127.0.0.1"
	}
	port := in.Port
	if port == 0 {
		port = 443
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
	name := displayName(u, nodeName)
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s",
		u.UUID, host, port, q.Encode(), url.QueryEscape(name))
}

func displayName(u models.User, nodeName string) string {
	base := u.Email
	if u.Remark != "" {
		base = u.Remark
	}
	if nodeName != "" {
		if base == "" {
			return nodeName
		}
		return nodeName + " · " + base
	}
	return base
}

func LinksForEndpoints(eps []Endpoint, u models.User) []string {
	var out []string
	for _, ep := range eps {
		in := models.Inbound{
			Port:       ep.Port,
			Flow:       ep.Flow,
			ServerName: ep.ServerName,
			PublicKey:  ep.PublicKey,
			ShortID:    ep.ShortID,
		}
		out = append(out, VLESSLinkNamed(ep.Host, in, u, ep.Name))
	}
	return out
}

func ClashYAML(host string, in models.Inbound, u models.User) string {
	return ClashYAMLMulti([]Endpoint{{
		Name:       displayName(u, ""),
		Host:       host,
		Port:       in.Port,
		Flow:       in.Flow,
		ServerName: in.ServerName,
		PublicKey:  in.PublicKey,
		ShortID:    in.ShortID,
	}}, u)
}

func ClashYAMLMulti(eps []Endpoint, u models.User) string {
	if len(eps) == 0 {
		return "proxies: []\nrules:\n  - MATCH,DIRECT\n"
	}
	var b strings.Builder
	b.WriteString("mixed-port: 7890\n")
	b.WriteString("allow-lan: false\n")
	b.WriteString("mode: rule\n")
	b.WriteString("proxies:\n")
	var names []string
	for _, ep := range eps {
		name := displayName(u, ep.Name)
		names = append(names, name)
		host := ep.Host
		if host == "" {
			host = "127.0.0.1"
		}
		port := ep.Port
		if port == 0 {
			port = 443
		}
		sni := ep.ServerName
		b.WriteString("  - name: " + strconv.Quote(name) + "\n")
		b.WriteString("    type: vless\n")
		b.WriteString("    server: " + host + "\n")
		b.WriteString("    port: " + strconv.Itoa(port) + "\n")
		b.WriteString("    uuid: " + u.UUID + "\n")
		b.WriteString("    network: tcp\n")
		b.WriteString("    tls: true\n")
		b.WriteString("    udp: true\n")
		b.WriteString("    flow: " + ep.Flow + "\n")
		b.WriteString("    servername: " + sni + "\n")
		b.WriteString("    client-fingerprint: chrome\n")
		b.WriteString("    reality-opts:\n")
		b.WriteString("      public-key: " + ep.PublicKey + "\n")
		b.WriteString("      short-id: " + ep.ShortID + "\n")
	}
	b.WriteString("proxy-groups:\n")
	b.WriteString("  - name: PROXY\n")
	b.WriteString("    type: select\n")
	b.WriteString("    proxies:\n")
	for _, name := range names {
		b.WriteString("      - " + strconv.Quote(name) + "\n")
	}
	b.WriteString("rules:\n")
	b.WriteString("  - MATCH,PROXY\n")
	return b.String()
}

func Base64VLESS(host string, in models.Inbound, u models.User) string {
	link := VLESSLink(host, in, u)
	return base64.StdEncoding.EncodeToString([]byte(link + "\n"))
}

func Base64Links(links []string) string {
	if len(links) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n") + "\n"))
}

func SubURL(publicURL, token string) string {
	base := strings.TrimRight(publicURL, "/")
	return base + "/sub/" + token
}
