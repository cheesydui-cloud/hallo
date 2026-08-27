package xray

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func (m *Manager) stopLocked() {
	if m.cmd != nil && m.cmd.Process != nil {
		pid := m.cmd.Process.Pid
		_ = syscall.Kill(-pid, syscall.SIGTERM)
		_ = m.cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			_, _ = m.cmd.Process.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			_ = m.cmd.Process.Kill()
			<-done
		}
	}
	m.cmd = nil
	m.killLeftoverLocked()
	waitPortsFree(portsFromConfigFile(m.cfg), 2*time.Second)
}

func (m *Manager) killLeftoverLocked() {
	self := os.Getpid()
	cfg := m.cfg
	if cfg == "" {
		return
	}
	for _, pid := range xrayPIDsUsingConfig(cfg) {
		if pid <= 1 || pid == self {
			continue
		}
		_ = syscall.Kill(pid, syscall.SIGTERM)
		_ = syscall.Kill(-pid, syscall.SIGTERM)
	}
	time.Sleep(250 * time.Millisecond)
	for _, pid := range xrayPIDsUsingConfig(cfg) {
		if pid <= 1 || pid == self {
			continue
		}
		_ = syscall.Kill(pid, syscall.SIGKILL)
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
}

func xrayPIDsUsingConfig(cfg string) []int {
	cfg = filepath.Clean(cfg)
	var out []int
	for _, p := range listProcesses() {
		if p.pid <= 1 {
			continue
		}
		cmd := strings.ToLower(p.cmd)
		if !strings.Contains(cmd, "xray") {
			continue
		}
		if strings.Contains(p.cmd, cfg) || strings.Contains(p.cmd, filepath.Base(cfg)) && strings.Contains(p.cmd, "run") {
			out = append(out, p.pid)
			continue
		}
		if strings.Contains(cmd, "xray") && strings.Contains(cmd, "-c") && strings.Contains(p.cmd, filepath.Dir(cfg)) {
			out = append(out, p.pid)
		}
	}
	return out
}

type procInfo struct {
	pid int
	cmd string
}

func listProcesses() []procInfo {
	if runtime.GOOS == "linux" {
		return listLinuxProcs()
	}
	return listPS()
}

func listLinuxProcs() []procInfo {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return listPS()
	}
	var out []procInfo
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil || len(raw) == 0 {
			continue
		}
		cmd := strings.ReplaceAll(string(raw), "\x00", " ")
		out = append(out, procInfo{pid: pid, cmd: strings.TrimSpace(cmd)})
	}
	return out
}

func listPS() []procInfo {
	out, err := exec.Command("ps", "-ax", "-o", "pid=,command=").CombinedOutput()
	if err != nil {
		return nil
	}
	var list []procInfo
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		list = append(list, procInfo{pid: pid, cmd: strings.Join(fields[1:], " ")})
	}
	return list
}

func listenTCP(port int) (net.Listener, error) {
	return net.Listen("tcp", ":"+strconv.Itoa(port))
}

func portsFromConfigFile(path string) []int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cfg struct {
		Inbounds []struct {
			Port int `json:"port"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil
	}
	seen := map[int]bool{}
	var ports []int
	for _, in := range cfg.Inbounds {
		if in.Port > 0 && !seen[in.Port] {
			seen[in.Port] = true
			ports = append(ports, in.Port)
		}
	}
	return ports
}

func waitPortsFree(ports []int, d time.Duration) {
	if len(ports) == 0 {
		return
	}
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		busy := false
		for _, p := range ports {
			if portBusy(p) {
				busy = true
				break
			}
		}
		if !busy {
			return
		}
		time.Sleep(80 * time.Millisecond)
	}
}

func portBusy(port int) bool {
	c, err := listenTCP(port)
	if err != nil {
		return true
	}
	_ = c.Close()
	return false
}

func isAddrInUse(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "address already in use") || strings.Contains(s, "bind: address already in use")
}

func friendlyXrayErr(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if isAddrInUse(err) {
		port := extractListenPort(msg)
		if port != "" {
			return "端口 " + port + " 已被占用。已尝试结束后台残留的 Xray。若仍失败，这台机器上可能还有 Nginx/Caddy/其他面板占着该端口，请换一个入站端口。"
		}
		return "入站端口已被占用。已尝试结束后台残留的 Xray。请换端口，或关掉占用该端口的其他程序。"
	}
	return msg
}

func extractListenPort(msg string) string {
	for _, sep := range []string{"0.0.0.0:", "::: ", "tcp ", "TCP on "} {
		if i := strings.LastIndex(strings.ToLower(msg), strings.ToLower(sep)); i >= 0 {
			rest := msg[i+len(sep):]
			n := ""
			for _, r := range rest {
				if r >= '0' && r <= '9' {
					n += string(r)
				} else if n != "" {
					break
				}
			}
			if n != "" {
				return n
			}
		}
	}
	return ""
}
