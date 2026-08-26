# Hallo

自研单节点代理面板（第 1 期）。独立实现，不依赖任何第三方商业面板的激活码或二进制。

一期能做的事：

- 浏览器里初始化管理员、管用户 / 套餐
- 本机 Xray：一条 **VLESS + Reality** 入站
- 每用户订阅：通用 VLESS 链接、Clash YAML
- CLI：`hallo serve` / `hallo plan add` / `hallo user add`

后续分期见仓库计划：第 2 期流量 Stats，第 3 期多节点 Agent，第 4 期支付。

## 本机开发（macOS）

需要 Go 1.22+、Node 18+。Xray 可选：没有也能先打开后台；有了才能真正转发。

```bash
make run
```

打开 http://127.0.0.1:18080 ，首次创建管理员。开发机入站端口建议 `18443`（不抢 443）。

手动分步：

```bash
cd web && npm install && npm run build && cd ..
go build -o bin/hallo ./cmd/hallo
HALLO_DEV=1 ./bin/hallo serve --listen :18080 --data data
```

前端热更新（另开终端）：

```bash
# 先启动面板
./bin/hallo serve
cd web && npm run dev   # http://127.0.0.1:5173 ，API 会代理到 :18080
```

## CLI

```bash
./bin/hallo plan add --name admin --limit 0 --days 0 --note "admin自用"
./bin/hallo user add --email alice --plan admin --remark "自己用"
./bin/hallo plan list
./bin/hallo user list
./bin/hallo xray reload
```

`--limit` 可以是字节，或 `10g` / `100m`。`0` 表示不限流量。

## Linux 部署

1. 把 `hallo` 放到 `/usr/local/bin/hallo`
2. 安装 [Xray-core 官方 release](https://github.com/XTLS/Xray-core/releases)，例如 `/usr/local/bin/xray`
3. 在设置页填 xray 路径，或环境变量 `HALLO_XRAY=/usr/local/bin/xray`
4. 生产入站用 443 需要 root 或 `setcap cap_net_bind_service=+ep`
5. 面板默认 `:18080`。前面建议再挂一层 Caddy/Nginx HTTPS，并把「公网地址」改成 `https://你的域名`

systemd 示例：

```ini
[Unit]
Description=Hallo panel
After=network-online.target

[Service]
ExecStart=/usr/local/bin/hallo serve --listen :18080 --data /var/lib/hallo
Environment=HALLO_XRAY=/usr/local/bin/xray
Restart=always
StateDirectory=hallo

[Install]
WantedBy=multi-user.target
```

## 数据目录

默认 `./data`：

- `hallo.db` SQLite
- `xray/config.json` 由面板生成，不要手改

## Reality 说明

- `dest` 必须是真实可访问的 TLS 网站（默认 `www.microsoft.com:443`）
- 密钥用本机 `xray x25519` 生成；初始化时若找不到 xray，会先写入占位，之后在「入站」页点重新生成
- 没有公网 IP / 域名时，订阅里的主机名来自设置里的「公网地址」
