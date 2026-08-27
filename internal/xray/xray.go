package xray

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"hallo/internal/models"
)

type Manager struct {
	mu   sync.Mutex
	cmd  *exec.Cmd
	bin  string
	cfg  string
	last string
}

func New(bin, cfgPath string) *Manager {
	return &Manager{bin: bin, cfg: cfgPath}
}

func (m *Manager) Bin() string { return m.bin }

func (m *Manager) SetBin(p string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bin = p
}

func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runningLocked()
}

func (m *Manager) runningLocked() bool {
	if m.cmd == nil || m.cmd.Process == nil {
		return false
	}
	err := m.cmd.Process.Signal(syscall.Signal(0))
	return err == nil
}

func (m *Manager) Message() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runningLocked() {
		return "运行中"
	}
	if m.bin == "" {
		return "未配置 xray 路径"
	}
	if _, err := os.Stat(m.bin); err != nil {
		return "未找到 xray 可执行文件，可在设置中填写路径，或把官方 xray 放到该位置"
	}
	if m.last != "" {
		return m.last
	}
	return "未启动"
}

func (m *Manager) WriteConfig(in models.Inbound, users []models.User) error {
	return m.WriteConfigBytes(BuildConfig(in, users, nil))
}

func (m *Manager) WriteConfigBytes(cfg map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(m.cfg), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.cfg, raw, 0o600)
}

func MarshalConfig(cfg map[string]any) ([]byte, error) {
	return json.MarshalIndent(cfg, "", "  ")
}

func (m *Manager) Reload() error {
	m.mu.Lock()
	if m.bin == "" {
		m.last = "未配置 xray 路径"
		m.mu.Unlock()
		return fmt.Errorf("xray path empty")
	}
	if _, err := os.Stat(m.bin); err != nil {
		m.last = "未找到 xray"
		m.mu.Unlock()
		return err
	}
	if m.runningLocked() {
		_ = m.cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			_, _ = m.cmd.Process.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = m.cmd.Process.Kill()
			<-done
		}
		m.cmd = nil
	}
	var stderr bytes.Buffer
	cmd := exec.Command(m.bin, "run", "-c", m.cfg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	if err := cmd.Start(); err != nil {
		m.last = err.Error()
		m.mu.Unlock()
		return err
	}
	m.cmd = cmd
	m.last = ""
	m.mu.Unlock()
	go func() {
		err := cmd.Wait()
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.cmd == cmd {
			m.cmd = nil
			msg := lastLogLine(stderr.String())
			if err != nil {
				m.last = err.Error()
				if msg != "" {
					m.last = m.last + " · " + msg
				}
			} else if msg != "" {
				m.last = msg
			} else {
				m.last = "已退出"
			}
		}
	}()
	time.Sleep(350 * time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.runningLocked() {
		if m.last == "" {
			m.last = lastLogLine(stderr.String())
		}
		if m.last == "" {
			m.last = "Xray 启动后立即退出，请检查端口是否被占用"
		}
		return fmt.Errorf("%s", m.last)
	}
	return nil
}

func lastLogLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			if len(line) > 240 {
				return line[len(line)-240:]
			}
			return line
		}
	}
	return ""
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runningLocked() {
		_ = m.cmd.Process.Signal(syscall.SIGTERM)
	}
}

func DefaultBin(dataDir string) string {
	name := "xray"
	if runtime.GOOS == "windows" {
		name = "xray.exe"
	}
	if dataDir == "" {
		dataDir = "data"
	}
	candidates := make([]string, 0, 8)
	if p := os.Getenv("HALLO_XRAY"); p != "" {
		candidates = append(candidates, p)
	}
	if p, err := exec.LookPath("xray"); err == nil {
		candidates = append(candidates, p)
	}
	candidates = append(candidates,
		"/usr/local/bin/xray",
		"/usr/bin/xray",
		filepath.Join(dataDir, "xray", name),
	)
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	if p := os.Getenv("HALLO_XRAY"); p != "" {
		return p
	}
	return filepath.Join(dataDir, "xray", name)
}

func IsPlaceholderKey(s string) bool {
	s = strings.TrimSpace(s)
	switch s {
	case "", "CHANGE_ME_PRIVATE", "CHANGE_ME_PUBLIC":
		return true
	}
	return false
}

func ValidRealityKey(s string) bool {
	if IsPlaceholderKey(s) {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(s))
	return err == nil && len(raw) == 32
}

func GenerateRealityKeys(bin string) (priv, pub string, err error) {
	if bin != "" {
		if _, statErr := os.Stat(bin); statErr == nil {
			out, runErr := exec.Command(bin, "x25519").CombinedOutput()
			if runErr == nil {
				priv, pub = parseX25519(string(out))
				if ValidRealityKey(priv) && ValidRealityKey(pub) {
					return priv, pub, nil
				}
			}
		}
	}
	return generateRealityKeysNative()
}

func generateRealityKeysNative() (priv, pub string, err error) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("生成 Reality 密钥失败：%w", err)
	}
	priv = base64.RawURLEncoding.EncodeToString(key.Bytes())
	pub = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
	return priv, pub, nil
}

func parseX25519(out string) (priv, pub string) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		low := strings.ToLower(line)
		switch {
		case strings.HasPrefix(low, "private"):
			priv = strings.TrimSpace(line[strings.Index(line, ":")+1:])
		case strings.HasPrefix(low, "public"):
			pub = strings.TrimSpace(line[strings.Index(line, ":")+1:])
		}
	}
	return
}

func RandomShortID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type Relay struct {
	Address    string
	Port       int
	UUID       string
	Flow       string
	PublicKey  string
	ShortID    string
	ServerName string
}

func BuildConfig(in models.Inbound, users []models.User, relay *Relay) map[string]any {
	return BuildFull([]models.Inbound{in}, users, nil, relay)
}

func BuildFull(inbounds []models.Inbound, users []models.User, outs []models.Outbound, relay *Relay) map[string]any {
	xIn := make([]any, 0, len(inbounds))
	inTags := make([]string, 0, len(inbounds))
	usedTag := map[string]int{}
	for i, in := range inbounds {
		if !in.Enabled && in.ID != 0 {
			continue
		}
		tag := strings.TrimSpace(in.Tag)
		if tag == "" {
			tag = "vless-in"
		}
		if usedTag[tag] > 0 {
			tag = fmt.Sprintf("%s-%d", tag, in.ID)
			if in.ID == 0 {
				tag = fmt.Sprintf("%s-%d", tag, i+1)
			}
		}
		usedTag[tag]++
		inTags = append(inTags, tag)
		xIn = append(xIn, buildVLESSInbound(in, tag, users))
	}
	if len(xIn) == 0 && len(inbounds) > 0 {
		in := inbounds[0]
		xIn = append(xIn, buildVLESSInbound(in, "vless-in", users))
		inTags = []string{"vless-in"}
	}

	outbounds, defaultTag := buildOutbounds(outs)
	if relay != nil && relay.Address != "" && relay.UUID != "" && relay.Port > 0 {
		sni := ""
		flow := ""
		if len(inbounds) > 0 {
			sni = inbounds[0].ServerName
			flow = inbounds[0].Flow
		}
		rsni := relay.ServerName
		if rsni == "" {
			rsni = sni
		}
		vnext := map[string]any{
			"address": relay.Address,
			"port":    relay.Port,
			"users": []any{
				map[string]any{
					"id":         relay.UUID,
					"encryption": "none",
					"flow":       firstNonEmpty(relay.Flow, flow),
				},
			},
		}
		outbounds = append([]any{
			map[string]any{
				"tag":      "relay",
				"protocol": "vless",
				"settings": map[string]any{"vnext": []any{vnext}},
				"streamSettings": map[string]any{
					"network":  "tcp",
					"security": "reality",
					"realitySettings": map[string]any{
						"serverName":  rsni,
						"fingerprint": "chrome",
						"publicKey":   relay.PublicKey,
						"shortId":     relay.ShortID,
					},
				},
			},
		}, outbounds...)
		defaultTag = "relay"
	}
	if defaultTag == "" {
		defaultTag = "direct"
	}
	rules := []any{}
	if len(inTags) > 0 {
		rules = append(rules, map[string]any{"type": "field", "inboundTag": inTags, "outboundTag": defaultTag})
	}
	return map[string]any{
		"log":       map[string]any{"loglevel": "warning"},
		"inbounds":  xIn,
		"outbounds": outbounds,
		"routing": map[string]any{
			"domainStrategy": "AsIs",
			"rules":          rules,
		},
	}
}

func buildVLESSInbound(in models.Inbound, tag string, users []models.User) map[string]any {
	clients := make([]map[string]any, 0, len(users))
	for _, u := range users {
		c := map[string]any{
			"id":    u.UUID,
			"email": u.Email,
		}
		if in.Flow != "" {
			c["flow"] = in.Flow
		}
		clients = append(clients, c)
	}
	listen := in.Listen
	if listen == "" {
		listen = "0.0.0.0"
	}
	port := in.Port
	if port == 0 {
		port = 443
	}
	dest := in.Dest
	if dest == "" {
		dest = "www.microsoft.com:443"
	}
	sni := in.ServerName
	if sni == "" {
		sni = strings.Split(dest, ":")[0]
	}
	return map[string]any{
		"tag":      tag,
		"listen":   listen,
		"port":     port,
		"protocol": firstNonEmpty(in.Protocol, "vless"),
		"settings": map[string]any{
			"clients":    clients,
			"decryption": "none",
		},
		"streamSettings": map[string]any{
			"network":  "tcp",
			"security": "reality",
			"realitySettings": map[string]any{
				"show":        false,
				"dest":        dest,
				"xver":        0,
				"serverNames": []string{sni},
				"privateKey":  in.PrivateKey,
				"shortIds":    []string{"", in.ShortID},
			},
		},
		// Vision + sniffing 改写目标会把连接掐断；routeOnly 只给路由看，不改出口。
		"sniffing": map[string]any{
			"enabled":      true,
			"destOverride": []string{"http", "tls"},
			"routeOnly":    true,
		},
	}
}

func buildOutbounds(outs []models.Outbound) ([]any, string) {
	if len(outs) == 0 {
		return []any{
			map[string]any{"protocol": "freedom", "tag": "direct"},
			map[string]any{"protocol": "blackhole", "tag": "block"},
		}, "direct"
	}
	var list []any
	defaultTag := ""
	seen := map[string]bool{}
	for _, o := range outs {
		if !o.Enabled {
			continue
		}
		tag := strings.TrimSpace(o.Tag)
		if tag == "" {
			tag = o.Protocol
		}
		if seen[tag] {
			tag = fmt.Sprintf("%s-%d", tag, o.ID)
		}
		seen[tag] = true
		item := outboundJSON(o, tag)
		if item == nil {
			continue
		}
		list = append(list, item)
		if o.IsDefault || defaultTag == "" {
			defaultTag = tag
		}
	}
	if !seen["direct"] {
		list = append(list, map[string]any{"protocol": "freedom", "tag": "direct"})
		if defaultTag == "" {
			defaultTag = "direct"
		}
	}
	if !seen["block"] {
		list = append(list, map[string]any{"protocol": "blackhole", "tag": "block"})
	}
	if defaultTag == "" {
		defaultTag = "direct"
	}
	return list, defaultTag
}

func outboundJSON(o models.Outbound, tag string) map[string]any {
	switch strings.ToLower(o.Protocol) {
	case "freedom", "direct":
		return map[string]any{"protocol": "freedom", "tag": tag}
	case "blackhole", "block":
		return map[string]any{"protocol": "blackhole", "tag": tag}
	case "vless":
		if o.Address == "" || o.Port <= 0 || o.UUID == "" {
			return nil
		}
		user := map[string]any{"id": o.UUID, "encryption": "none"}
		if o.Flow != "" {
			user["flow"] = o.Flow
		}
		item := map[string]any{
			"tag":      tag,
			"protocol": "vless",
			"settings": map[string]any{
				"vnext": []any{
					map[string]any{
						"address": o.Address,
						"port":    o.Port,
						"users":   []any{user},
					},
				},
			},
		}
		if o.PublicKey != "" {
			sni := o.ServerName
			if sni == "" {
				sni = o.Address
			}
			item["streamSettings"] = map[string]any{
				"network":  "tcp",
				"security": "reality",
				"realitySettings": map[string]any{
					"serverName":  sni,
					"fingerprint": "chrome",
					"publicKey":   o.PublicKey,
					"shortId":     o.ShortID,
				},
			}
		}
		return item
	case "socks", "http":
		if o.Address == "" || o.Port <= 0 {
			return nil
		}
		server := map[string]any{"address": o.Address, "port": o.Port}
		if o.Username != "" {
			server["users"] = []any{map[string]any{"user": o.Username, "pass": o.Password}}
		}
		return map[string]any{
			"tag":      tag,
			"protocol": strings.ToLower(o.Protocol),
			"settings": map[string]any{"servers": []any{server}},
		}
	default:
		return map[string]any{"protocol": o.Protocol, "tag": tag}
	}
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func ParseLimitBytes(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" || s == "0" || s == "unlimited" {
		return 0, nil
	}
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "g"):
		mult = 1 << 30
		s = strings.TrimSuffix(s, "g")
	case strings.HasSuffix(s, "gb"):
		mult = 1 << 30
		s = strings.TrimSuffix(s, "gb")
	case strings.HasSuffix(s, "m"):
		mult = 1 << 20
		s = strings.TrimSuffix(s, "m")
	case strings.HasSuffix(s, "mb"):
		mult = 1 << 20
		s = strings.TrimSuffix(s, "mb")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, err
	}
	return n * mult, nil
}
