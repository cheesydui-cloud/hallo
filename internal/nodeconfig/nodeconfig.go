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

func inboundsForNode(database *db.DB, n models.Node) ([]models.Inbound, error) {
	items, err := database.ListInboundsForNode(n.ID)
	if err != nil {
		return nil, err
	}
	var enabled []models.Inbound
	for _, in := range items {
		if !in.Enabled && in.ID != 0 {
			continue
		}
		if in.Port == 0 && n.Port > 0 {
			in.Port = n.Port
		}
		enabled = append(enabled, in)
	}
	return enabled, nil
}

func Build(database *db.DB, n models.Node, in models.Inbound) (map[string]any, error) {
	inbounds, err := inboundsForNode(database, n)
	if err != nil {
		return nil, err
	}
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
		targetIn, _ := database.ListInboundsForNode(dst.ID)
		pub, sid, sni, flow := in.PublicKey, in.ShortID, in.ServerName, in.Flow
		port := dst.Port
		for _, ti := range targetIn {
			if !ti.Enabled {
				continue
			}
			pub = ti.PublicKey
			sid = ti.ShortID
			sni = ti.ServerName
			flow = ti.Flow
			if ti.Port > 0 {
				port = ti.Port
			}
			break
		}
		if port == 0 {
			port = 443
		}
		relay = &xray.Relay{
			Address:    addr,
			Port:       port,
			UUID:       n.RelayUUID,
			Flow:       flow,
			PublicKey:  pub,
			ShortID:    sid,
			ServerName: sni,
		}
	}
	outs, _ := database.ListOutboundsForNode(n.ID)
	return xray.BuildFull(inbounds, users, outs, relay), nil
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
		ins, err := database.ListInboundsForNode(n.ID)
		if err != nil {
			continue
		}
		for _, ib := range ins {
			if !ib.Enabled {
				continue
			}
			port := ib.Port
			if port == 0 {
				port = n.Port
			}
			if port == 0 {
				port = in.Port
			}
			if port == 0 {
				port = 443
			}
			name := n.Name
			if strings.TrimSpace(ib.Remark) != "" && ib.Remark != n.Name {
				name = n.Name + "-" + ib.Remark
			}
			eps = append(eps, sub.Endpoint{
				Name:       name,
				Host:       host,
				Port:       port,
				Flow:       firstNonEmpty(ib.Flow, in.Flow),
				ServerName: firstNonEmpty(ib.ServerName, in.ServerName),
				PublicKey:  firstNonEmpty(ib.PublicKey, in.PublicKey),
				ShortID:    firstNonEmpty(ib.ShortID, in.ShortID),
			})
		}
	}
	return eps, nil
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
