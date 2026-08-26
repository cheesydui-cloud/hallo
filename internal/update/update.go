package update

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const DefaultRepo = "cheesydui-cloud/hallo"

type Release struct {
	Tag     string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
	Notes   string  `json:"body"`
}

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type Status struct {
	Current     string            `json:"current"`
	Latest      string            `json:"latest"`
	Newer       bool              `json:"newer"`
	HTMLURL     string            `json:"html_url"`
	Repo        string            `json:"repo"`
	Arch        string            `json:"arch"`
	Asset       string            `json:"asset"`
	AgentStaged map[string]string `json:"agent_staged"`
}

func Arch() string {
	switch runtime.GOARCH {
	case "amd64", "arm64":
		return runtime.GOARCH
	default:
		return runtime.GOARCH
	}
}

func AssetName(role, goos, arch string) string {
	if role == "agent" {
		return fmt.Sprintf("hallo-agent-%s-%s.tar.gz", goos, arch)
	}
	return fmt.Sprintf("hallo-%s-%s.tar.gz", goos, arch)
}

func FetchLatest(repo string) (*Release, error) {
	if repo == "" {
		repo = DefaultRepo
	}
	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "hallo-panel")
	cli := &http.Client{Timeout: 20 * time.Second}
	res, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return nil, fmt.Errorf("GitHub %s: %s", res.Status, strings.TrimSpace(string(b)))
	}
	var rel Release
	if err := json.NewDecoder(res.Body).Decode(&rel); err != nil {
		return nil, err
	}
	if rel.Tag == "" {
		return nil, fmt.Errorf("GitHub 未返回 tag")
	}
	return &rel, nil
}

func FindAsset(rel *Release, name string) (*Asset, error) {
	for i := range rel.Assets {
		if rel.Assets[i].Name == name {
			return &rel.Assets[i], nil
		}
	}
	return nil, fmt.Errorf("Release %s 没有附件 %s", rel.Tag, name)
}

func Newer(latest, current string) bool {
	l := strings.TrimPrefix(strings.TrimSpace(latest), "v")
	c := strings.TrimPrefix(strings.TrimSpace(current), "v")
	if c == "" || c == "dev" {
		return l != ""
	}
	return cmpVer(l, c) > 0
}

func cmpVer(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var ai, bi int
		if i < len(as) {
			fmt.Sscanf(as[i], "%d", &ai)
		}
		if i < len(bs) {
			fmt.Sscanf(bs[i], "%d", &bi)
		}
		if ai > bi {
			return 1
		}
		if ai < bi {
			return -1
		}
	}
	return 0
}

func Download(url, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "hallo-panel")
	cli := &http.Client{Timeout: 5 * time.Minute}
	res, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("下载失败 %s", res.Status)
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, res.Body); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

func ExtractFile(tgz, want, dest string) error {
	f, err := os.Open(tgz)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.Base(hdr.Name)
		if name != want || hdr.Typeflag != tar.TypeReg {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		tmp := dest + ".new"
		out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			_ = os.Remove(tmp)
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		return os.Rename(tmp, dest)
	}
	return fmt.Errorf("压缩包里没有 %s", want)
}

func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func ReplaceExec(src, dest string) error {
	if dest == "" {
		p, err := os.Executable()
		if err != nil {
			return err
		}
		dest, err = filepath.EvalSymlinks(p)
		if err != nil {
			dest = p
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp := dest + ".new"
	if err := os.WriteFile(tmp, raw, 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

func RestartPanel() error {
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		cmd := exec.Command("systemctl", "restart", "hallo")
		return cmd.Start()
	}
	return fmt.Errorf("未检测到 systemd，请手动重启 hallo serve")
}

func RestartAgent() error {
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		cmd := exec.Command("systemctl", "restart", "hallo-agent")
		return cmd.Start()
	}
	return fmt.Errorf("未检测到 systemd，请手动重启 hallo-agent")
}

func SelfPath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r, nil
	}
	return p, nil
}
