#!/bin/sh
# Hallo 一键安装 / 升级（Linux systemd）
# 安装：
#   curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sudo sh
# 指定版本：
#   curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sudo sh -s -- --version v0.1.0
# 升级（只换二进制，保留 /etc/hallo 与数据库）：
#   curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sudo sh -s -- --upgrade
# 可选：
#   --listen :18080 --public-url http://IP:18080 --skip-xray
set -eu

REPO="${HALLO_REPO:-cheesydui-cloud/hallo}"
CDN="${HALLO_CDN:-https://github.com/${REPO}/releases}"
VERSION=""
UPGRADE=0
LISTEN=":18080"
PUBLIC_URL=""
SKIP_XRAY=0
XRAY_VERSION="${HALLO_XRAY_VERSION:-}"

while [ $# -gt 0 ]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --upgrade) UPGRADE=1; shift ;;
    --listen) LISTEN="$2"; shift 2 ;;
    --public-url) PUBLIC_URL="$2"; shift 2 ;;
    --skip-xray) SKIP_XRAY=1; shift ;;
    --xray-version) XRAY_VERSION="$2"; shift 2 ;;
    -h|--help)
      cat <<'EOF'
Hallo 一键安装 / 升级（Linux systemd）

  curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sudo sh
  curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sudo sh -s -- --version v0.1.0
  curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sudo sh -s -- --upgrade

可选：--listen :18080 --public-url http://IP:18080 --skip-xray
EOF
      exit 0
      ;;
    *)
      echo "未知参数：$1" >&2
      exit 2
      ;;
  esac
done

if [ "$(id -u)" != "0" ]; then
  echo "请用 root 运行，例如：curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sudo sh" >&2
  exit 1
fi

if ! command -v systemctl >/dev/null 2>&1; then
  echo "需要 systemd。" >&2
  exit 1
fi

detect_arch() {
  m=$(uname -m)
  case "$m" in
    x86_64|amd64) echo amd64 ;;
    aarch64|arm64) echo arm64 ;;
    *) echo "不支持的架构：$m（需要 x86_64 或 aarch64）" >&2; exit 1 ;;
  esac
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "缺少命令：$1" >&2
    exit 1
  fi
}

need_cmd curl
need_cmd tar

ARCH=$(detect_arch)
if [ -z "$VERSION" ]; then
  VERSION=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/${REPO}/releases/latest")
  VERSION=${VERSION##*/}
fi
if [ -z "$VERSION" ] || [ "$VERSION" = "latest" ] || [ "$VERSION" = "releases" ]; then
  echo "无法解析最新版本，请加 --version v0.1.0" >&2
  exit 1
fi
case "$VERSION" in
  v*) ;;
  *) VERSION="v$VERSION" ;;
esac

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

asset="hallo-linux-${ARCH}.tar.gz"
url="${CDN}/download/${VERSION}/${asset}"
echo "下载 Hallo ${VERSION}（${ARCH}） $url"
if ! curl -fL --connect-timeout 15 --max-time 300 -o "$tmpdir/$asset" "$url"; then
  echo "下载失败。确认已发布 ${VERSION}，或改用 --version。" >&2
  exit 1
fi
if curl -fsSL --connect-timeout 10 --max-time 60 -o "$tmpdir/$asset.sha256" "${url}.sha256" 2>/dev/null; then
  (cd "$tmpdir" && sha256sum -c "$asset.sha256")
fi
tar -xzf "$tmpdir/$asset" -C "$tmpdir"
if [ ! -x "$tmpdir/hallo" ]; then
  echo "压缩包里没有 hallo 二进制" >&2
  exit 1
fi
	install -m 0755 "$tmpdir/hallo" /usr/local/bin/hallo

stage_agents() {
  mkdir -p /var/lib/hallo/agents
  for a in amd64 arm64; do
    aname="hallo-agent-linux-${a}.tar.gz"
    aurl="${CDN}/download/${VERSION}/${aname}"
    echo "暂存 agent ${a} $aurl"
    if ! curl -fL --connect-timeout 15 --max-time 300 -o "$tmpdir/$aname" "$aurl"; then
      echo "跳过 $aname（Release 里可能还没有）"
      continue
    fi
    mkdir -p "$tmpdir/agent-$a"
    tar -xzf "$tmpdir/$aname" -C "$tmpdir/agent-$a"
    if [ -f "$tmpdir/agent-$a/hallo-agent" ]; then
      install -m 0755 "$tmpdir/agent-$a/hallo-agent" "/var/lib/hallo/agents/hallo-agent-linux-$a"
      echo "已暂存 /var/lib/hallo/agents/hallo-agent-linux-$a"
    fi
  done
}

	install_xray() {
  if [ "$SKIP_XRAY" = 1 ]; then
    echo "跳过 Xray（--skip-xray）"
    return 0
  fi
  xray_arch="64"
  [ "$ARCH" = "arm64" ] && xray_arch="arm64-v8a"
  if [ -z "$XRAY_VERSION" ]; then
    XRAY_VERSION=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/XTLS/Xray-core/releases/latest")
    XRAY_VERSION=${XRAY_VERSION##*/}
  fi
  if [ -z "$XRAY_VERSION" ]; then
    echo "无法解析 Xray 版本，跳过自动安装。可稍后把官方 xray 放到 /usr/local/bin/xray" >&2
    return 0
  fi
  zip="Xray-linux-${xray_arch}.zip"
  xurl="https://github.com/XTLS/Xray-core/releases/download/${XRAY_VERSION}/${zip}"
  echo "下载 Xray ${XRAY_VERSION} $xurl"
  if ! command -v unzip >/dev/null 2>&1; then
    echo "没有 unzip，跳过 Xray 安装。请手动安装官方 Xray-core。" >&2
    return 0
  fi
  if ! curl -fL --connect-timeout 15 --max-time 300 -o "$tmpdir/$zip" "$xurl"; then
    echo "下载 Xray 失败，跳过。面板仍可打开，之后在设置里填写 xray 路径。" >&2
    return 0
  fi
  unzip -qo "$tmpdir/$zip" -d "$tmpdir/xray"
  if [ -f "$tmpdir/xray/xray" ]; then
    install -m 0755 "$tmpdir/xray/xray" /usr/local/bin/xray
    echo "已安装 /usr/local/bin/xray"
  fi
}

write_env() {
  mkdir -p /etc/hallo /var/lib/hallo
  chmod 0750 /var/lib/hallo
  {
    printf 'HALLO_LISTEN=%s\n' "$LISTEN"
    printf 'HALLO_DATA=/var/lib/hallo\n'
    printf 'HALLO_XRAY=/usr/local/bin/xray\n'
    [ -n "$PUBLIC_URL" ] && printf 'HALLO_PUBLIC_URL=%s\n' "$PUBLIC_URL"
  } >/etc/hallo/hallo.env
  chmod 0644 /etc/hallo/hallo.env
}

write_unit() {
  cat >/etc/systemd/system/hallo.service <<'UNIT'
[Unit]
Description=Hallo panel
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=-/etc/hallo/hallo.env
ExecStart=/usr/local/bin/hallo serve
Restart=always
RestartSec=2
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
UNIT
}

	install_xray
	stage_agents

	if [ "$UPGRADE" = 1 ]; then
	  systemctl daemon-reload
	  systemctl restart hallo.service
  echo "Hallo 已升级到 ${VERSION} 并重启（配置与数据库未改）"
  echo "版本：$(/usr/local/bin/hallo version 2>/dev/null || echo "$VERSION")"
  exit 0
fi

write_env
write_unit
systemctl daemon-reload
systemctl enable hallo.service >/dev/null
systemctl restart hallo.service || true
sleep 1

echo ""
echo "Hallo ${VERSION} 已安装"
echo "  二进制：/usr/local/bin/hallo"
echo "  数据：  /var/lib/hallo"
echo "  配置：  /etc/hallo/hallo.env"
echo "  监听：  ${LISTEN}"
if systemctl is-active --quiet hallo.service; then
  echo "  服务：  运行中"
else
  echo "  服务未起来，看日志：journalctl -u hallo -n 50 --no-pager"
fi
echo ""
echo "浏览器打开 http://服务器IP${LISTEN} 创建管理员。"
echo "生产环境请在前面挂 HTTPS，并把设置里的公网地址改成 https://域名"
echo ""
echo "升级："
echo "  curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sudo sh -s -- --upgrade"
echo "给管理员加不限量套餐："
echo "  hallo plan add --name admin --limit 0 --days 0 --note \"admin自用\""
echo "  hallo user add --email admin --plan admin --remark \"自己用\""
