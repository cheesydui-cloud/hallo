package xray

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func EnsureBinary(dest string) (string, error) {
	if dest == "" {
		dest = DefaultBin("")
	}
	if st, err := os.Stat(dest); err == nil && !st.IsDir() && st.Size() > 1024 {
		return dest, nil
	}
	if p, err := LookPathFallback(); err == nil {
		return p, nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if err := downloadOfficial(dest); err != nil {
		return "", err
	}
	return dest, nil
}

func LookPathFallback() (string, error) {
	for _, p := range []string{"/usr/local/bin/xray", "/usr/bin/xray", DefaultBin("")} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("xray not found")
}

func downloadOfficial(dest string) error {
	arch := "64"
	switch runtime.GOARCH {
	case "arm64", "aarch64":
		arch = "arm64-v8a"
	case "amd64", "x86_64":
		arch = "64"
	default:
		return fmt.Errorf("不支持自动安装 Xray 的架构：%s", runtime.GOARCH)
	}
	if runtime.GOOS != "linux" {
		return fmt.Errorf("自动安装 Xray 仅支持 Linux，当前 %s", runtime.GOOS)
	}
	cli := &http.Client{Timeout: 3 * time.Minute}
	req, err := http.NewRequest(http.MethodHead, "https://github.com/XTLS/Xray-core/releases/latest", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "hallo")
	res, err := cli.Do(req)
	if err != nil {
		return err
	}
	res.Body.Close()
	loc := res.Request.URL.String()
	ver := loc[strings.LastIndex(loc, "/")+1:]
	if ver == "" || ver == "latest" {
		return fmt.Errorf("无法解析 Xray 版本")
	}
	url := "https://github.com/XTLS/Xray-core/releases/download/" + ver + "/Xray-linux-" + arch + ".zip"
	tmp, err := os.CreateTemp("", "xray-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	zr, err := cli.Get(url)
	if err != nil {
		tmp.Close()
		return err
	}
	defer zr.Body.Close()
	if zr.StatusCode != 200 {
		tmp.Close()
		return fmt.Errorf("下载 Xray 失败：%s", zr.Status)
	}
	if _, err := io.Copy(tmp, zr.Body); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	r, err := zip.OpenReader(tmp.Name())
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		base := filepath.Base(f.Name)
		if base != "xray" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		return err
	}
	return fmt.Errorf("zip 里没有 xray")
}
