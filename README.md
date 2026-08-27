# Hallo

自研多节点代理面板。独立实现，不依赖任何第三方商业面板。面板和 Agent 会拉取官方 [Xray-core](https://github.com/XTLS/Xray-core) 到本机并拉起进程，不是把核心编进 Go 二进制。

**就三步：**

1. **服务器** — 本机已经在跑 Xray。其它机器登记后，把安装命令拿到那台机用 root 执行，等到「在线」且 Xray 运行中
2. **协议** — 左边点那台服务器，右边添加 VLESS / VMess / Shadowsocks；点「复制」导入客户端
3. **客户端** — 初始化已经有一个默认 UUID。只有要多个 UUID 时才来这里加

当前版本：

- 左侧栏只留：服务器 / 协议 / 客户端 / 设置
- 每台服务器只下发自己的入站；改美国机不会因为本机 443 被占用而失败
- 协议页可直接复制可用分享链接（默认客户端 UUID 已自动创建）
- 面板自更新、Agent 一键安装 / 推送更新

---

## 一键安装（Linux 服务器）

需要：root、systemd、x86_64 或 aarch64。脚本会安装 `hallo`、官方 `xray`，并写入 systemd。

已经是 root 时不要再套一层 `sudo`。

**安装（始终装当前仓库最新 Release）：**

```bash
curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sh
```

**指定版本：**

```bash
curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sh -s -- --version v0.7.0
```

**自定义面板端口 / 公网地址：**

```bash
curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sh -s -- \
  --listen :18080 \
  --public-url http://你的服务器IP:18080
```

装完浏览器打开：

```text
http://服务器IP:18080
```

第一次启动会让你创建管理员。生产默认入站端口是 **443**（需要 root）。只有本机开发、没有 443 权限时才填 `18443`，或设 `HALLO_DEV=1`。

### 升级（推荐从面板点）

打开 **设置 → 检查更新 → 一键更新面板**。面板会向 GitHub Release 拉当前架构的包、替换 `/usr/local/bin/hallo` 并重启，同时把新的 `hallo-agent` 暂存到本机，供节点下载。

```bash
curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sh -s -- --upgrade
```

钉死版本：

```bash
curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sh -s -- --upgrade --version v0.7.0
```

升级完后，到 **服务器** 点「一键推送全部 Agent」，让远程机换新包。

---

## 服务器怎么用

面板地址（`:18080`）**不是**代理入口。客户端连的是那台服务器的 **公网 IP/域名 + 协议端口（默认 443）**。

1. 设置里填好面板 **公网地址**（节点机能访问）
2. **服务器** 页会出现「本机」。把本机公网 IP 填进公网地址
3. 再加远程机器：名字 + 公网 IP。复制安装命令，到**那台机 root** 执行：

```bash
curl -fsSL 'http://面板IP:18080/install/agent.sh?token=节点token' | sh
```

成功时节点机会打印 `hallo-agent 已安装并在 systemd 中运行`，并尽量安装官方 Xray。面板里该节点变成「在线」，Agent 心跳会拉 Xray 配置并启动。

4. 打开 **协议**：左边点刚上线的服务器，点「复制」导入客户端。没有协议就点「添加协议」
5. 同一台机端口不能重复。443 被 nginx / 旧 xray 占用时，换 8443 / 2053 等

不要用前台的 `hallo-agent run`：关掉终端进程就没了。

---

## 装完常用命令

```bash
systemctl status hallo
journalctl -u hallo -n 80 --no-pager
hallo version

HALLO_DATA=/var/lib/hallo hallo xray reload
```

| 路径 | 作用 |
|------|------|
| `/usr/local/bin/hallo` | 面板 |
| `/usr/local/bin/hallo-agent` | 节点 Agent |
| `/usr/local/bin/xray` | 官方核心 |
| `/etc/hallo/hallo.env` | 面板环境 |
| `/etc/hallo/agent.env` | Agent 环境 |
| `/var/lib/hallo/hallo.db` | SQLite |
| `/var/lib/hallo/xray/config.json` | 本机 Xray 配置（面板生成） |
| `/var/lib/hallo-agent/xray/config.json` | 节点 Xray 配置（Agent 生成） |

生产请在面板前面挂 Caddy/Nginx HTTPS，并在「设置」里把公网地址改成 `https://你的域名`。订阅里的 **代理主机名** 来自各节点的公网地址，不是这个面板域名。

---

## 本机开发（macOS）

需要 Go 1.22+、Node 18+。没有 Xray 也能先打开后台。

```bash
make run
```

打开 http://127.0.0.1:18080 。开发机入站端口建议 `18443`（`HALLO_DEV=1`）。

```bash
cd web && npm install && npm run build && cd ..
go test ./...
go build -o bin/hallo ./cmd/hallo
HALLO_DEV=1 ./bin/hallo serve --listen :18080 --data data
```

发版：

```bash
sh scripts/release.sh v0.7.0
```

打 Git 标签 `v*` 会触发 GitHub Actions，自动出 Release 附件。

## Reality 说明

- `dest` 必须是真实可访问的 TLS 网站（默认 `www.microsoft.com:443`）
- 密钥由面板直接生成；打开协议页会自动替换掉旧的 `CHANGE_ME_*` 占位符
- 客户端 `pbk` / `sid` / `sni` / 端口都在「复制」出来的链接里，不必手填
