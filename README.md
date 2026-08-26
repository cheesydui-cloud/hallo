# Hallo

自研多节点代理面板。独立实现，不依赖任何第三方商业面板的激活码或二进制。Xray **不是**内嵌在面板里的，安装脚本和 Agent 会拉取官方 [Xray-core](https://github.com/XTLS/Xray-core)。

当前版本能做的事：

- 浏览器初始化管理员、管用户 / 套餐 / 节点
- 本机 + 远程节点都跑 **VLESS + Reality**
- 用户可选节点；订阅按节点公网地址出多条链接（通用 VLESS + Clash YAML）
- 链式转发：入口节点把流量转到另一台节点再出网
- Reality 密钥由面板用 X25519 生成，不依赖 `xray x25519`
- 面板自更新、Agent 一键安装 / 推送更新
- CLI：`hallo serve` / `hallo plan add` / `hallo user add`

流量精确 Stats 仍是下一期。

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
curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sh -s -- --version v0.2.0
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
curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sh -s -- --upgrade --version v0.2.0
```

---

## 节点怎么用（订阅到底连哪台）

面板地址（`:18080`）**不是**代理入口。客户端连的是节点的 **公网 IP/域名 + 入站端口（默认 443）**。

1. 设置里填好面板 **公网地址**（节点机能访问）
2. **节点** 页会出现「本机」。把本机公网 IP 填进该节点的公网地址（初始化时若已填面板公网地址会自动带上）
3. 再加远程节点：名字 + 公网 IP + 端口。链式转发可选「转到哪一台」
4. 复制安装命令，到**节点机 root** 执行：

```bash
curl -fsSL 'http://面板IP:18080/install/agent.sh?token=节点token' | sh
```

成功时节点机会打印 `hallo-agent 已安装并在 systemd 中运行`，并尽量安装官方 Xray。面板里该节点变成「在线」，Agent 心跳会拉 Xray 配置并启动。

5. **用户** 页可不选节点（订阅包含全部启用节点），或只勾要用的节点
6. 复制用户的「订阅」给客户端。Clash 订阅里每台节点一条，可在客户端里选

不要用前台的 `hallo-agent run`：关掉终端进程就没了。

### 链式转发

在节点编辑里把「链式转发到」设成另一台节点。入口机会用 VLESS+Reality 把流量转到目标机；目标机上会自动多一个中转 UUID。客户端仍然只连入口节点的公网地址。

---

## 装完常用命令

```bash
systemctl status hallo
journalctl -u hallo -n 80 --no-pager
hallo version

HALLO_DATA=/var/lib/hallo hallo plan add --name admin --limit 0 --days 0 --note "admin自用"
HALLO_DATA=/var/lib/hallo hallo user add --email admin --plan admin --remark "自己用"
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
sh scripts/release.sh v0.2.0
```

打 Git 标签 `v*` 会触发 GitHub Actions，自动出 Release 附件。

## Reality 说明

- `dest` 必须是真实可访问的 TLS 网站（默认 `www.microsoft.com:443`）
- 密钥由面板直接生成；打开入站页会自动替换掉旧的 `CHANGE_ME_*` 占位符
- 客户端 `pbk` / `sid` / `sni` 来自入站页；`address` / `port` 来自节点页
