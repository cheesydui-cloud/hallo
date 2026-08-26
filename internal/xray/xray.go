package xray

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	mu     sync.Mutex
	cmd    *exec.Cmd
	bin    string
	cfg    string
	last   string
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
	if err := os.MkdirAll(filepath.Dir(m.cfg), 0o755); err != nil {
		return err
	}
	cfg := buildConfig(in, users)
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.cfg, raw, 0o600)
}

func (m *Manager) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.bin == "" {
		m.last = "未配置 xray 路径"
		return fmt.Errorf("xray path empty")
	}
	if _, err := os.Stat(m.bin); err != nil {
		m.last = "未找到 xray"
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
	cmd := exec.Command(m.bin, "run", "-c", m.cfg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		m.last = err.Error()
		return err
	}
	m.cmd = cmd
	m.last = ""
	go func() {
		err := cmd.Wait()
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.cmd == cmd {
			m.cmd = nil
			if err != nil {
				m.last = err.Error()
			} else {
				m.last = "已退出"
			}
		}
	}()
	return nil
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runningLocked() {
		_ = m.cmd.Process.Signal(syscall.SIGTERM)
	}
}

func DefaultBin(dataDir string) string {
	if p := os.Getenv("HALLO_XRAY"); p != "" {
		return p
	}
	if p, err := exec.LookPath("xray"); err == nil {
		return p
	}
	name := "xray"
	if runtime.GOOS == "windows" {
		name = "xray.exe"
	}
	if dataDir == "" {
		dataDir = "data"
	}
	return filepath.Join(dataDir, "xray", name)
}

func GenerateRealityKeys(bin string) (priv, pub string, err error) {
	if bin != "" {
		if _, statErr := os.Stat(bin); statErr == nil {
			out, runErr := exec.Command(bin, "x25519").CombinedOutput()
			if runErr == nil {
				priv, pub = parseX25519(string(out))
				if priv != "" && pub != "" {
					return priv, pub, nil
				}
			}
		}
	}
	// Fallback placeholder keys are invalid for real clients; operator should
	// generate via `xray x25519` once the binary is present.
	return "", "", fmt.Errorf("需要 xray 可执行文件来生成 Reality 密钥（xray x25519）")
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

func buildConfig(in models.Inbound, users []models.User) map[string]any {
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
		"log": map[string]any{"loglevel": "warning"},
		"inbounds": []any{
			map[string]any{
				"listen":   listen,
				"port":     port,
				"protocol": "vless",
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
						"shortIds":    []string{in.ShortID},
					},
				},
				"sniffing": map[string]any{
					"enabled":      true,
					"destOverride": []string{"http", "tls", "quic"},
				},
			},
		},
		"outbounds": []any{
			map[string]any{"protocol": "freedom", "tag": "direct"},
			map[string]any{"protocol": "blackhole", "tag": "block"},
		},
	}
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
