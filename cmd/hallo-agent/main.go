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
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"hallo/internal/agentproto"
	"hallo/internal/update"
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

  hallo-agent run --panel URL --node-id ID --token TOKEN
  hallo-agent version

环境变量：HALLO_PANEL HALLO_NODE_ID HALLO_TOKEN
`, version)
}

func run(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	panel := fs.String("panel", os.Getenv("HALLO_PANEL"), "面板地址，如 http://1.2.3.4:18080")
	nodeID := fs.String("node-id", os.Getenv("HALLO_NODE_ID"), "节点 ID")
	token := fs.String("token", os.Getenv("HALLO_TOKEN"), "节点 token")
	interval := fs.Duration("interval", 30*time.Second, "心跳间隔")
	_ = fs.Parse(args)
	if *panel == "" || *nodeID == "" || *token == "" {
		log.Fatal("需要 --panel --node-id --token")
	}

	host, _ := os.Hostname()
	cli := &http.Client{Timeout: 30 * time.Second}

	go beat(*panel, *nodeID, *token, host, cli)
	tk := time.NewTicker(*interval)
	defer tk.Stop()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case <-tk.C:
			go beat(*panel, *nodeID, *token, host, cli)
		case <-ch:
			return
		}
	}
}

func beat(panel, nodeID, token, host string, cli *http.Client) {
	body, _ := json.Marshal(agentproto.Heartbeat{
		NodeID:  nodeID,
		Token:   token,
		Version: version,
		Arch:    runtime.GOARCH,
		OS:      runtime.GOOS,
		Host:    host,
	})
	req, err := http.NewRequest(http.MethodPost, trim(panel)+"/api/agent/heartbeat", bytes.NewReader(body))
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
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != 200 {
		log.Printf("heartbeat %s: %s", res.Status, raw)
		return
	}
	var reply agentproto.HeartbeatReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		log.Printf("heartbeat decode: %v", err)
		return
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
		log.Printf("重启：%v（若由 systemd 管理会在进程退出后拉起）", err)
		os.Exit(0)
	}
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
		// 面板 /download/agent 直接给二进制
		if copyErr := copyFile(tgz, bin); copyErr != nil {
			return err
		}
		_ = os.Chmod(bin, 0o755)
	}
	self, err := update.SelfPath()
	if err != nil {
		return err
	}
	return update.ReplaceExec(bin, self)
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

func trim(s string) string {
	if len(s) == 0 {
		return s
	}
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
