package nodeconfig

import (
	"fmt"
	"net"
	"strings"

	"hallo/internal/db"
	"hallo/internal/models"
	"hallo/internal/sub"
	"hallo/internal/xray"
)

func NodeAddress(n models.Node, fallback string) string {
	for _, c := range []string{n.PublicHost, n.Host} {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if host, _, err := net.SplitHostPort(c); err == nil && host != "" {
			return host
		}
		if i := strings.Index(c, "://"); i >= 0 {
			c = c[i+3:]
		}
		c = strings.Split(c, "/")[0]
		if host, _, err := net.SplitHostPort(c); err == nil && host != "" {
			return host
		}
		if c != "" {
			return c
		}
	}
	return fallback
}

func InboundForNode(base models.Inbound, n models.Node) models.Inbound {
	in := base
	if n.Port > 0 {
		in.Port = n.Port
	}
	return in
}

func Build(database *db.DB, n models.Node, in models.Inbound) (map[string]any, error) {
	users, err := database.UsersForNode(n.ID)
	if err != nil {
		return nil, err
	}
	others, err := database.ListNodes()
	if err != nil {
		return nil, err
	}
	for _, o := range others {
		if o.RelayNodeID != nil && *o.RelayNodeID == n.ID && strings.TrimSpace(o.RelayUUID) != "" {
			users = append(users, models.User{
				Email:   "relay@" + o.Name,
				UUID:    o.RelayUUID,
				Enabled: true,
			})
		}
	}
	var relay *xray.Relay
	if n.RelayNodeID != nil && *n.RelayNodeID > 0 && *n.RelayNodeID != n.ID {
		dst, err := database.GetNode(*n.RelayNodeID)
		if err != nil {
			return nil, fmt.Errorf("链式目标节点不存在")
		}
		addr := NodeAddress(*dst, "")
		if addr == "" {
			return nil, fmt.Errorf("链式目标「%s」还没有公网地址", dst.Name)
		}
		if strings.TrimSpace(n.RelayUUID) == "" {
			return nil, fmt.Errorf("链式转发缺少中转 UUID")
		}
		relay = &xray.Relay{
			Address:    addr,
			Port:       dst.Port,
			UUID:       n.RelayUUID,
			Flow:       in.Flow,
			PublicKey:  in.PublicKey,
			ShortID:    in.ShortID,
			ServerName: in.ServerName,
		}
	}
	cfgIn := InboundForNode(in, n)
	return xray.BuildConfig(cfgIn, users, relay), nil
}

func Endpoints(database *db.DB, u models.User, in models.Inbound, fallbackHost string) ([]sub.Endpoint, error) {
	nodes, err := database.ListNodes()
	if err != nil {
		return nil, err
	}
	allow := map[int64]bool{}
	restrict := len(u.NodeIDs) > 0
	for _, id := range u.NodeIDs {
		allow[id] = true
	}
	var eps []sub.Endpoint
	for _, n := range nodes {
		if !n.Enabled || !n.Subscribe {
			continue
		}
		if restrict && !allow[n.ID] {
			continue
		}
		host := NodeAddress(n, "")
		if host == "" {
			if n.IsLocal {
				host = fallbackHost
			} else {
				continue
			}
		}
		if host == "" {
			continue
		}
		port := n.Port
		if port == 0 {
			port = in.Port
		}
		eps = append(eps, sub.Endpoint{
			Name:       n.Name,
			Host:       host,
			Port:       port,
			Flow:       in.Flow,
			ServerName: in.ServerName,
			PublicKey:  in.PublicKey,
			ShortID:    in.ShortID,
		})
	}
	return eps, nil
}
