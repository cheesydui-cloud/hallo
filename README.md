# Hallo

自研单节点代理面板（第 1 期）。独立实现，不依赖任何第三方商业面板的激活码或二进制。

一期能做的事：

- 浏览器里初始化管理员、管用户 / 套餐
- 本机 Xray：一条 **VLESS + Reality** 入站
- 每用户订阅：通用 VLESS 链接、Clash YAML
- CLI：`hallo serve` / `hallo plan add` / `hallo user add`

后续：第 2 期流量 Stats，第 3 期多节点流量与分流，第 4 期支付。面板内已可登记 Agent 并一键推送更新。

---

## 一键安装（Linux 服务器）

需要：root、systemd、x86_64 或 aarch64。脚本会安装 `hallo`、官方 `xray`，并写入 systemd。

**安装（始终装当前仓库最新 Release）：**

```bash
curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sudo sh
```

**指定版本：**

```bash
curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sudo sh -s -- --version v0.1.0
```

**自定义面板端口 / 公网地址：**

```bash
curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sudo sh -s -- \
  --listen :18080 \
  --public-url http://你的服务器IP:18080
```

**只装面板、不自动拉 Xray：**

```bash
curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sudo sh -s -- --skip-xray
```

装完浏览器打开：

```text
http://服务器IP:18080
```

第一次启动会让你创建管理员。开发/无 443 权限时，入站端口填 `18443`；生产有 root 再用 `443`。

### 升级（推荐从面板点）

打开 **设置 → 检查更新 → 一键更新面板**。面板会向 GitHub Release 拉当前架构的包、替换 `/usr/local/bin/hallo` 并重启，同时把新的 `hallo-agent` 暂存到本机，供节点下载。

命令行升级（保留数据和配置）：

```bash
curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sudo sh -s -- --upgrade
```

钉死版本：

```bash
curl -fsSL https://raw.githubusercontent.com/cheesydui-cloud/hallo/main/scripts/install.sh | sudo sh -s -- --upgrade --version v0.1.0
```

### Agent 一键推送

1. 设置里填好 **公网地址**（节点要能访问这块面板）
2. **节点** 页添加节点，把安装命令拿到节点机执行
3. 点该节点的 **推送更新**，或 **一键推送全部 Agent**
4. 节点下次心跳（约 30 秒）会从 `http://面板/download/agent/<arch>` 拉新包并重启

不需要 SSH 上每台机器跑升级脚本。

### 装完常用命令

```bash
# 状态 / 日志
systemctl status hallo
journalctl -u hallo -n 80 --no-pager

# 版本
hallo version

# 给管理员加不限量套餐（数据目录要和 systemd 一致）
HALLO_DATA=/var/lib/hallo hallo plan add --name admin --limit 0 --days 0 --note "admin自用"
HALLO_DATA=/var/lib/hallo hallo user add --email admin --plan admin --remark "自己用"
HALLO_DATA=/var/lib/hallo hallo plan list
HALLO_DATA=/var/lib/hallo hallo user list

# 重载 Xray
HALLO_DATA=/var/lib/hallo hallo xray reload
```

文件位置：

| 路径 | 作用 |
|------|------|
| `/usr/local/bin/hallo` | 面板 |
| `/usr/local/bin/xray` | 官方核心 |
| `/etc/hallo/hallo.env` | 监听地址、数据目录 |
| `/var/lib/hallo/hallo.db` | SQLite |
| `/var/lib/hallo/xray/config.json` | 由面板生成，不要手改 |
| `/etc/systemd/system/hallo.service` | 服务 |

生产请在面板前面挂 Caddy/Nginx HTTPS，并在「设置」里把公网地址改成 `https://你的域名`，订阅里的主机名才正确。

---

## 本机开发（macOS）

需要 Go 1.22+、Node 18+。没有 Xray 也能先打开后台。

```bash
make run
```

打开 http://127.0.0.1:18080 ，首次创建管理员。开发机入站端口建议 `18443`。

手动分步：

```bash
cd web && npm install && npm run build && cd ..
go build -o bin/hallo ./cmd/hallo
HALLO_DEV=1 ./bin/hallo serve --listen :18080 --data data
```

前端热更新（另开终端）：

```bash
./bin/hallo serve
cd web && npm run dev
```

发版打包（本机交叉编译 Linux 包）：

```bash
sh scripts/release.sh v0.1.0
```

打 Git 标签 `v*` 会触发 GitHub Actions，自动出 Release 附件。

## Reality 说明

- `dest` 必须是真实可访问的 TLS 网站（默认 `www.microsoft.com:443`）
- 密钥用本机 `xray x25519` 生成；初始化时若找不到 xray，会先写入占位，之后在「入站」页点重新生成
- 订阅里的主机名来自设置里的「公网地址」
