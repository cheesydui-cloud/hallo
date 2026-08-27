package sub

import (
	"encoding/base64"
	"encoding/json"
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
	Protocol   string
	Flow       string
	Security   string
	ServerName string
	PublicKey  string
	ShortID    string
	Method     string
	Password   string
}

func VLESSLink(host string, in models.Inbound, u models.User) string {
	return ShareLink(Endpoint{
		Host:       host,
		Port:       in.Port,
		Protocol:   firstNonEmpty(in.Protocol, "vless"),
		Flow:       in.Flow,
		Security:   in.Security,
		ServerName: in.ServerName,
		PublicKey:  in.PublicKey,
		ShortID:    in.ShortID,
		Method:     in.Method,
		Password:   in.Password,
	}, u, "")
}

func VLESSLinkNamed(host string, in models.Inbound, u models.User, nodeName string) string {
	return ShareLink(Endpoint{
		Name:       nodeName,
		Host:       host,
		Port:       in.Port,
		Protocol:   firstNonEmpty(in.Protocol, "vless"),
		Flow:       in.Flow,
		Security:   in.Security,
		ServerName: in.ServerName,
		PublicKey:  in.PublicKey,
		ShortID:    in.ShortID,
		Method:     in.Method,
		Password:   in.Password,
	}, u, nodeName)
}

func ShareLink(ep Endpoint, u models.User, nodeName string) string {
	host := strings.TrimSpace(ep.Host)
	if host == "" {
		return ""
	}
	port := ep.Port
	if port == 0 {
		port = 443
	}
	name := displayName(u, firstNonEmpty(nodeName, ep.Name))
	proto := strings.ToLower(strings.TrimSpace(ep.Protocol))
	switch proto {
	case "vmess":
		return vmessLink(host, port, u.UUID, name)
	case "shadowsocks", "ss":
		return ssLink(host, port, ep.Method, ep.Password, name)
	default:
		return vlessLink(host, port, ep, u.UUID, name)
	}
}

func vlessLink(host string, port int, ep Endpoint, uuid, name string) string {
	sni := ep.ServerName
	q := url.Values{}
	q.Set("encryption", "none")
	q.Set("type", "tcp")
	if ep.Security == "none" {
		q.Set("security", "none")
	} else {
		q.Set("security", "reality")
		q.Set("sni", sni)
		q.Set("fp", "chrome")
		q.Set("pbk", ep.PublicKey)
		q.Set("sid", ep.ShortID)
		if ep.Flow != "" {
			q.Set("flow", ep.Flow)
		}
	}
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s", uuid, host, port, q.Encode(), url.QueryEscape(name))
}

func vmessLink(host string, port int, uuid, name string) string {
	obj := map[string]any{
		"v":    "2",
		"ps":   name,
		"add":  host,
		"port": strconv.Itoa(port),
		"id":   uuid,
		"aid":  "0",
		"net":  "tcp",
		"type": "none",
		"tls":  "",
	}
	raw, _ := json.Marshal(obj)
	return "vmess://" + base64.StdEncoding.EncodeToString(raw)
}

func ssLink(host string, port int, method, password, name string) string {
	if method == "" {
		method = "aes-128-gcm"
	}
	user := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString([]byte(method + ":" + password))
	return fmt.Sprintf("ss://%s@%s:%d#%s", user, host, port, url.QueryEscape(name))
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
		out = append(out, ShareLink(ep, u, ep.Name))
	}
	return out
}

func ClashYAML(host string, in models.Inbound, u models.User) string {
	return ClashYAMLMulti([]Endpoint{{
		Name:       displayName(u, ""),
		Host:       host,
		Port:       in.Port,
		Protocol:   firstNonEmpty(in.Protocol, "vless"),
		Flow:       in.Flow,
		Security:   in.Security,
		ServerName: in.ServerName,
		PublicKey:  in.PublicKey,
		ShortID:    in.ShortID,
		Method:     in.Method,
		Password:   in.Password,
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
		proto := strings.ToLower(strings.TrimSpace(ep.Protocol))
		b.WriteString("  - name: " + strconv.Quote(name) + "\n")
		switch proto {
		case "vmess":
			b.WriteString("    type: vmess\n")
			b.WriteString("    server: " + host + "\n")
			b.WriteString("    port: " + strconv.Itoa(port) + "\n")
			b.WriteString("    uuid: " + u.UUID + "\n")
			b.WriteString("    alterId: 0\n")
			b.WriteString("    cipher: auto\n")
			b.WriteString("    network: tcp\n")
			b.WriteString("    udp: true\n")
		case "shadowsocks", "ss":
			method := ep.Method
			if method == "" {
				method = "aes-128-gcm"
			}
			b.WriteString("    type: ss\n")
			b.WriteString("    server: " + host + "\n")
			b.WriteString("    port: " + strconv.Itoa(port) + "\n")
			b.WriteString("    cipher: " + method + "\n")
			b.WriteString("    password: " + strconv.Quote(ep.Password) + "\n")
			b.WriteString("    udp: true\n")
		default:
			b.WriteString("    type: vless\n")
			b.WriteString("    server: " + host + "\n")
			b.WriteString("    port: " + strconv.Itoa(port) + "\n")
			b.WriteString("    uuid: " + u.UUID + "\n")
			b.WriteString("    network: tcp\n")
			b.WriteString("    udp: true\n")
			if ep.Security == "none" {
				b.WriteString("    tls: false\n")
			} else {
				b.WriteString("    tls: true\n")
				b.WriteString("    flow: " + ep.Flow + "\n")
				b.WriteString("    servername: " + ep.ServerName + "\n")
				b.WriteString("    client-fingerprint: chrome\n")
				b.WriteString("    reality-opts:\n")
				b.WriteString("      public-key: " + ep.PublicKey + "\n")
				b.WriteString("      short-id: " + ep.ShortID + "\n")
			}
		}
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

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
