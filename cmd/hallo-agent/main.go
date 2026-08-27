package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"hallo/internal/agentproto"
	"hallo/internal/update"
	"hallo/internal/xray"
)

var version = "dev"

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("hallo-agent ")
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "run":
		run(os.Args[2:])
	case "install":
		install(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Println(version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "未知命令：%s\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Hallo Agent %s

  hallo-agent install --panel URL --token TOKEN [--node-id ID]
  hallo-agent run --panel URL --token TOKEN
  hallo-agent version

install 会写入 systemd、尽量安装官方 Xray，并启动。关掉 SSH 后进程仍在。
节点机会按面板下发的配置跑官方 Xray。
环境变量：HALLO_PANEL HALLO_NODE_ID HALLO_TOKEN HALLO_XRAY
`, version)
}

type agentRuntime struct {
	mu        sync.Mutex
	xm        *xray.Manager
	configRev string
	dataDir   string
}

func run(args []string) {
	panel, nodeID, token, interval := parseConnFlags("run", args)
	host, _ := os.Hostname()
	cli := &http.Client{Timeout: 45 * time.Second}
	data := "/var/lib/hallo-agent"
	if v := os.Getenv("HALLO_AGENT_DATA"); v != "" {
		data = v
	}
	_ = os.MkdirAll(data, 0o750)
	bin, err := xray.EnsureBinary(xray.DefaultBin(data))
	if err != nil {
		log.Printf("安装/查找 xray 失败：%v（仍会心跳，配置下发后会再试）", err)
		bin = xray.DefaultBin(data)
	} else {
		log.Printf("xray：%s", bin)
	}
	rt := &agentRuntime{
		xm:      xray.New(bin, filepath.Join(data, "xray", "config.json")),
		dataDir: data,
	}
	go rt.beat(panel, nodeID, token, host, cli)
	tk := time.NewTicker(interval)
	defer tk.Stop()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case <-tk.C:
			go rt.beat(panel, nodeID, token, host, cli)
		case <-ch:
			rt.xm.Stop()
			return
		}
	}
}

func parseConnFlags(name string, args []string) (panel, nodeID, token string, interval time.Duration) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	p := fs.String("panel", os.Getenv("HALLO_PANEL"), "面板地址，如 http://1.2.3.4:18080")
	id := fs.String("node-id", os.Getenv("HALLO_NODE_ID"), "节点 ID（可选）")
	tok := fs.String("token", os.Getenv("HALLO_TOKEN"), "节点 token")
	iv := fs.Duration("interval", 20*time.Second, "心跳间隔")
	_ = fs.Parse(args)
	if strings.TrimSpace(*p) == "" || strings.TrimSpace(*tok) == "" {
		log.Fatal("需要 --panel 和 --token")
	}
	return strings.TrimRight(*p, "/"), *id, *tok, *iv
}

func install(args []string) {
	panel, nodeID, token, _ := parseConnFlags("install", args)
	if os.Geteuid() != 0 {
		log.Fatal("install 需要 root")
	}
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		log.Fatal("需要 systemd")
	}
	self, err := update.SelfPath()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll("/etc/hallo", 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll("/var/lib/hallo-agent", 0o750); err != nil {
		log.Fatal(err)
	}
	if bin, err := xray.EnsureBinary("/usr/local/bin/xray"); err != nil {
		log.Printf("自动安装 xray 失败：%v（服务仍会启动，稍后重试）", err)
	} else {
		log.Printf("xray：%s", bin)
	}
	env := fmt.Sprintf("HALLO_PANEL=%s\nHALLO_NODE_ID=%s\nHALLO_TOKEN=%s\nHALLO_XRAY=/usr/local/bin/xray\nHALLO_AGENT_DATA=/var/lib/hallo-agent\n", panel, nodeID, token)
	if err := os.WriteFile("/etc/hallo/agent.env", []byte(env), 0o600); err != nil {
		log.Fatal(err)
	}
	unit := `[Unit]
Description=Hallo agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=-/etc/hallo/agent.env
ExecStart=` + self + ` run
Restart=always
RestartSec=2
LimitNOFILE=1048576
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
`
	if err := os.WriteFile("/etc/systemd/system/hallo-agent.service", []byte(unit), 0o644); err != nil {
		log.Fatal(err)
	}
	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		log.Fatalf("daemon-reload: %v %s", err, out)
	}
	if out, err := exec.Command("systemctl", "enable", "--now", "hallo-agent.service").CombinedOutput(); err != nil {
		log.Fatalf("enable/start: %v %s", err, out)
	}
	time.Sleep(800 * time.Millisecond)
	if err := exec.Command("systemctl", "is-active", "--quiet", "hallo-agent.service").Run(); err != nil {
		out, _ := exec.Command("journalctl", "-u", "hallo-agent", "-n", "40", "--no-pager").CombinedOutput()
		log.Fatalf("服务未运行。日志：\n%s", out)
	}
	fmt.Println("hallo-agent 已安装并在 systemd 中运行")
	fmt.Println("  面板：", panel)
	fmt.Println("  查看：systemctl status hallo-agent")
	fmt.Println("  日志：journalctl -u hallo-agent -n 50 --no-pager")
}

func (rt *agentRuntime) beat(panel, nodeID, token, host string, cli *http.Client) {
	rt.mu.Lock()
	rev := rt.configRev
	running := rt.xm.Running()
	msg := rt.xm.Message()
	rt.mu.Unlock()

	body, _ := json.Marshal(agentproto.Heartbeat{
		NodeID:      nodeID,
		Token:       token,
		Version:     version,
		Arch:        runtime.GOARCH,
		OS:          runtime.GOOS,
		Host:        host,
		PublicIP:    publicIP(cli),
		XrayRunning: running,
		XrayMessage: msg,
		ConfigRev:   rev,
	})
	req, err := http.NewRequest(http.MethodPost, panel+"/api/agent/heartbeat", bytes.NewReader(body))
	if err != nil {
		log.Printf("heartbeat: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := cli.Do(req)
	if err != nil {
		log.Printf("heartbeat: %v", err)
		return
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if res.StatusCode != 200 {
		log.Printf("heartbeat %s: %s", res.Status, raw)
		return
	}
	var reply agentproto.HeartbeatReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		log.Printf("heartbeat decode: %v", err)
		return
	}
	if reply.Config != nil {
		if err := rt.applyConfig(reply.ConfigRev, reply.Config); err != nil {
			log.Printf("应用 Xray 配置失败：%v", err)
		} else {
			log.Printf("已应用配置 %s，Xray %s", reply.ConfigRev, rt.xm.Message())
		}
	}
	if !reply.ForceUpdate || reply.UpdateURL == "" {
		return
	}
	log.Printf("面板要求升级到 %s", reply.DesiredVer)
	if err := applyUpdate(reply.UpdateURL); err != nil {
		log.Printf("升级失败：%v", err)
		return
	}
	log.Printf("已写入新二进制，准备重启")
	if err := update.RestartAgent(); err != nil {
		log.Printf("重启：%v", err)
		os.Exit(0)
	}
}

func (rt *agentRuntime) applyConfig(rev string, cfg map[string]any) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if _, err := os.Stat(rt.xm.Bin()); err != nil {
		if bin, err := xray.EnsureBinary(rt.xm.Bin()); err == nil {
			rt.xm.SetBin(bin)
		} else {
			return err
		}
	}
	if err := rt.xm.WriteConfigBytes(cfg); err != nil {
		return err
	}
	if err := rt.xm.Reload(); err != nil {
		return err
	}
	rt.configRev = rev
	return nil
}

func publicIP(cli *http.Client) string {
	urls := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
	}
	c := *cli
	c.Timeout = 4 * time.Second
	for _, u := range urls {
		res, err := c.Get(u)
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(res.Body, 64))
		res.Body.Close()
		ip := strings.TrimSpace(string(b))
		if ip != "" && !strings.Contains(ip, " ") && !strings.Contains(ip, "<") {
			return ip
		}
	}
	return ""
}

func applyUpdate(url string) error {
	dir, err := os.MkdirTemp("", "hallo-agent-up-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	tgz := filepath.Join(dir, "pkg.tar.gz")
	if err := update.Download(url, tgz); err != nil {
		return err
	}
	bin := filepath.Join(dir, "hallo-agent")
	if err := update.ExtractFile(tgz, "hallo-agent", bin); err != nil {
		if copyErr := copyFile(tgz, bin); copyErr != nil {
			return err
		}
		_ = os.Chmod(bin, 0o755)
	}
	if err := mustELF(bin); err != nil {
		return err
	}
	self, err := update.SelfPath()
	if err != nil {
		return err
	}
	return update.ReplaceExec(bin, self)
}

func mustELF(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var magic [4]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		return err
	}
	if magic[0] != 0x7f || magic[1] != 'E' || magic[2] != 'L' || magic[3] != 'F' {
		return fmt.Errorf("不是 Linux 可执行文件（可能下到了 HTML 错误页）")
	}
	return nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
