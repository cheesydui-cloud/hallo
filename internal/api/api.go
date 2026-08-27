package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"hallo/internal/agentproto"
	"hallo/internal/auth"
	"hallo/internal/config"
	"hallo/internal/db"
	"hallo/internal/models"
	"hallo/internal/nodeconfig"
	"hallo/internal/sub"
	"hallo/internal/update"
	"hallo/internal/xray"
)

type Server struct {
	cfg     config.Config
	db      *db.DB
	xray    *xray.Manager
	web     fs.FS
	version string
	upMu    sync.Mutex
}

func New(cfg config.Config, database *db.DB, xm *xray.Manager, webFS fs.FS, version string) *Server {
	return &Server{cfg: cfg, db: database, xray: xm, web: webFS, version: version}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(5 * time.Minute))

	r.Get("/api/meta", s.meta)
	r.Post("/api/setup", s.setup)
	r.Post("/api/login", s.login)
	r.Post("/api/logout", s.logout)

	r.Get("/sub/{token}", s.subscription)
	r.Get("/sub/{token}/clash", s.subscriptionClash)
	r.Post("/api/agent/heartbeat", s.agentHeartbeat)
	r.Get("/download/agent/{arch}", s.downloadAgent)
	r.Get("/install/agent.sh", s.agentInstallScript)

	r.Group(func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Get("/api/dashboard", s.dashboard)
		r.Get("/api/plans", s.listPlans)
		r.Post("/api/plans", s.createPlan)
		r.Put("/api/plans/{id}", s.updatePlan)
		r.Delete("/api/plans/{id}", s.deletePlan)
		r.Get("/api/users", s.listUsers)
		r.Post("/api/users", s.createUser)
		r.Put("/api/users/{id}", s.updateUser)
		r.Post("/api/users/{id}/reset-traffic", s.resetTraffic)
		r.Post("/api/users/{id}/reset-uuid", s.resetUUID)
		r.Delete("/api/users/{id}", s.deleteUser)
		r.Get("/api/inbound", s.getInbound)
		r.Put("/api/inbound", s.putInbound)
		r.Post("/api/inbound/regen-keys", s.regenKeys)
		r.Get("/api/inbounds", s.listInbounds)
		r.Post("/api/inbounds", s.createInbound)
		r.Put("/api/inbounds/{id}", s.updateInbound)
		r.Delete("/api/inbounds/{id}", s.deleteInbound)
		r.Post("/api/inbounds/{id}/regen-keys", s.regenInboundKeys)
		r.Get("/api/outbounds", s.listOutbounds)
		r.Post("/api/outbounds", s.createOutbound)
		r.Put("/api/outbounds/{id}", s.updateOutbound)
		r.Delete("/api/outbounds/{id}", s.deleteOutbound)
		r.Post("/api/xray/reload", s.reloadXray)
		r.Get("/api/settings", s.getSettings)
		r.Put("/api/settings", s.putSettings)
		r.Get("/api/update", s.updateStatus)
		r.Post("/api/update", s.applyPanelUpdate)
		r.Get("/api/nodes", s.listNodes)
		r.Post("/api/nodes", s.createNode)
		r.Put("/api/nodes/{id}", s.updateNode)
		r.Delete("/api/nodes/{id}", s.deleteNode)
		r.Post("/api/nodes/{id}/push-update", s.pushNodeUpdate)
		r.Post("/api/nodes/push-update", s.pushAllNodeUpdates)
	})

	if s.web != nil {
		r.Handle("/*", spaHandler(s.web))
	}
	return r
}

func spaHandler(fsys fs.FS) http.Handler {
	files := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" || p == "." {
			p = "index.html"
		}
		if _, err := fs.Stat(fsys, p); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
			files.ServeHTTP(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	return dec.Decode(dst)
}

func (s *Server) cookieSecure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) setSession(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     "hallo_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cookieSecure(r),
		Expires:  expires,
	})
}

func (s *Server) sessionToken(r *http.Request) string {
	if c, err := r.Cookie("hallo_session"); err == nil {
		return c.Value
	}
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := s.sessionToken(r)
		if tok == "" {
			writeErr(w, http.StatusUnauthorized, "未登录")
			return
		}
		if _, err := s.db.SessionAdmin(tok); err != nil {
			writeErr(w, http.StatusUnauthorized, "登录已过期")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) meta(w http.ResponseWriter, r *http.Request) {
	n, err := s.db.AdminCount()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"setup_needed": n == 0,
		"name":         "Hallo",
		"version":      s.version,
	})
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	n, err := s.db.AdminCount()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if n > 0 {
		writeErr(w, 400, "已经初始化过了")
		return
	}
	var body struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		PublicURL string `json:"public_url"`
		Port      int    `json:"port"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	body.Username = strings.TrimSpace(body.Username)
	if body.Username == "" || len(body.Password) < 6 {
		writeErr(w, 400, "用户名不能为空，密码至少 6 位")
		return
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := s.db.CreateAdmin(body.Username, hash); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if _, err := s.db.CreatePlan(models.Plan{Name: "default", Note: "默认套餐（不限流量）"}); err != nil && !db.IsUniqueErr(err) {
		writeErr(w, 500, err.Error())
		return
	}
	if err := s.ensureInbound(body.Port); err != nil {
		writeErr(w, 500, "初始化入站失败："+err.Error())
		return
	}
	pub := strings.TrimSpace(body.PublicURL)
	if pub == "" {
		host, _, _ := net.SplitHostPort(r.Host)
		if host == "" {
			host = r.Host
		}
		scheme := "http"
		if s.cookieSecure(r) {
			scheme = "https"
		}
		pub = scheme + "://" + r.Host
	}
	_ = s.db.SetSetting("public_url", pub)
	_ = s.db.SetSetting("listen", s.cfg.Listen)
	_ = s.db.SetSetting("xray_path", s.xray.Bin())
	_ = s.ensureLocalNode(body.Port, pub)

	expires := time.Now().Add(7 * 24 * time.Hour)
	tok, err := auth.RandomToken(32)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	admin, _ := s.db.GetAdminByUsername(body.Username)
	if admin != nil {
		_ = s.db.CreateSession(tok, admin.ID, expires)
		s.setSession(w, r, tok, expires)
	}
	_ = s.syncXray()
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) defaultInboundPort() int {
	if s.cfg.Dev {
		return 18443
	}
	return 443
}

func (s *Server) ensureInbound(port int) error {
	if in, err := s.db.GetInbound(); err == nil {
		return s.repairInbound(in)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if port == 0 {
		port = s.defaultInboundPort()
	}
	priv, pub, err := xray.GenerateRealityKeys(s.xray.Bin())
	if err != nil {
		return err
	}
	in := &models.Inbound{
		Remark:     "本机 Reality",
		Tag:        "vless-in",
		Protocol:   "vless",
		Listen:     "0.0.0.0",
		Port:       port,
		Flow:       "xtls-rprx-vision",
		Security:   "reality",
		Dest:       "www.microsoft.com:443",
		ServerName: "www.microsoft.com",
		PrivateKey: priv,
		PublicKey:  pub,
		ShortID:    xray.RandomShortID(),
		Enabled:    true,
	}
	if local, err := s.db.LocalNode(); err == nil {
		in.NodeID = local.ID
		in.Remark = local.Name
	}
	return s.db.SaveInbound(in)
}

func (s *Server) repairInbound(in *models.Inbound) error {
	if in == nil {
		return nil
	}
	if strings.EqualFold(in.Protocol, "shadowsocks") || strings.EqualFold(in.Protocol, "ss") {
		method, password, err := xray.NormalizeSS(in.Method, in.Password)
		if err != nil {
			return err
		}
		if method != in.Method || password != in.Password {
			in.Method = method
			in.Password = password
			return s.db.SaveInbound(in)
		}
		return nil
	}
	if strings.ToLower(in.Protocol) != "vless" || in.Security == "none" {
		return nil
	}
	changed := false
	if xray.IsPlaceholderKey(in.PrivateKey) || xray.IsPlaceholderKey(in.PublicKey) ||
		!xray.ValidRealityKey(in.PrivateKey) || !xray.ValidRealityKey(in.PublicKey) {
		priv, pub, err := xray.GenerateRealityKeys(s.xray.Bin())
		if err != nil {
			return err
		}
		in.PrivateKey = priv
		in.PublicKey = pub
		changed = true
	}
	if strings.TrimSpace(in.ShortID) == "" {
		in.ShortID = xray.RandomShortID()
		changed = true
	}
	if changed {
		return s.db.SaveInbound(in)
	}
	return nil
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	a, err := s.db.GetAdminByUsername(strings.TrimSpace(body.Username))
	if err != nil || !auth.CheckPassword(a.PasswordHash, body.Password) {
		writeErr(w, 401, "用户名或密码错误")
		return
	}
	tok, err := auth.RandomToken(32)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	expires := time.Now().Add(7 * 24 * time.Hour)
	if err := s.db.CreateSession(tok, a.ID, expires); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.setSession(w, r, tok, expires)
	writeJSON(w, 200, map[string]any{"ok": true, "username": a.Username})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if tok := s.sessionToken(r); tok != "" {
		_ = s.db.DeleteSession(tok)
	}
	s.setSession(w, r, "", time.Unix(0, 0))
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	ut, ue, pt, tr, err := s.db.Stats()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	n, _ := s.db.AdminCount()
	in, _ := s.db.GetInbound()
	port := 0
	if in != nil {
		port = in.Port
	}
	nodes, _ := s.db.ListNodes()
	online := 0
	for _, nd := range nodes {
		if nd.Online || nd.IsLocal {
			online++
		}
	}
	writeJSON(w, 200, models.Dashboard{
		SetupNeeded:  n == 0,
		UserTotal:    ut,
		UserEnabled:  ue,
		PlanTotal:    pt,
		NodeTotal:    len(nodes),
		NodeOnline:   online,
		TrafficTotal: tr,
		XrayRunning:  s.xray.Running(),
		XrayPath:     s.xray.Bin(),
		XrayMessage:  s.xray.Message(),
		PublicURL:    s.db.GetSetting("public_url", ""),
		InboundPort:  port,
	})
}

func (s *Server) listPlans(w http.ResponseWriter, r *http.Request) {
	items, err := s.db.ListPlans()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *Server) createPlan(w http.ResponseWriter, r *http.Request) {
	var p models.Plan
	if err := readJSON(r, &p); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		writeErr(w, 400, "套餐名不能为空")
		return
	}
	id, err := s.db.CreatePlan(p)
	if err != nil {
		if db.IsUniqueErr(err) {
			writeErr(w, 409, "套餐名已存在")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	p.ID = id
	writeJSON(w, 200, p)
}

func (s *Server) updatePlan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var p models.Plan
	if err := readJSON(r, &p); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	if _, err := parseID(id); err != nil {
		writeErr(w, 400, "无效 id")
		return
	}
	var err error
	p.ID, err = parseID(id)
	if err != nil {
		writeErr(w, 400, "无效 id")
		return
	}
	if err := s.db.UpdatePlan(p); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, p)
}

func (s *Server) deletePlan(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, 400, "无效 id")
		return
	}
	if err := s.db.DeletePlan(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	items, err := s.db.ListUsers()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	fallback := sub.PublicHost(s.db.GetSetting("public_url", ""), hostname(r))
	in, _ := s.db.GetInbound()
	type item struct {
		models.User
		SubURL     string   `json:"sub_url"`
		VlessLink  string   `json:"vless_link"`
		VlessLinks []string `json:"vless_links"`
		ClashURL   string   `json:"clash_url"`
		PublicHost string   `json:"public_host"`
		Active     bool     `json:"active"`
	}
	out := make([]item, 0, len(items))
	pub := s.publicBase(r)
	for _, u := range items {
		it := item{User: u, PublicHost: fallback, SubURL: sub.SubURL(pub, u.SubToken), ClashURL: sub.SubURL(pub, u.SubToken) + "/clash", Active: s.db.UserActive(u)}
		if in != nil {
			eps, _ := nodeconfig.Endpoints(s.db, u, *in, fallback)
			links := sub.LinksForEndpoints(eps, u)
			it.VlessLinks = links
			if len(links) > 0 {
				it.VlessLink = links[0]
			}
		}
		out = append(out, it)
	}
	writeJSON(w, 200, map[string]any{"items": out})
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email   string  `json:"email"`
		Remark  string  `json:"remark"`
		PlanID  *int64  `json:"plan_id"`
		NodeIDs []int64 `json:"node_ids"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	body.Email = strings.TrimSpace(body.Email)
	if body.Email == "" {
		writeErr(w, 400, "邮箱/标识不能为空")
		return
	}
	u := models.User{
		Email:   body.Email,
		Remark:  strings.TrimSpace(body.Remark),
		UUID:    uuid.NewString(),
		PlanID:  body.PlanID,
		Enabled: true,
	}
	tok, err := auth.RandomToken(16)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	u.SubToken = tok
	if body.PlanID != nil {
		p, err := s.db.GetPlan(*body.PlanID)
		if err == nil && p.DurationDays > 0 {
			t := time.Now().Add(time.Duration(p.DurationDays) * 24 * time.Hour)
			u.ExpireAt = &t
		}
	}
	id, err := s.db.CreateUser(u)
	if err != nil {
		if db.IsUniqueErr(err) {
			writeErr(w, 409, "该标识已存在")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	u.ID = id
	if err := s.db.SetUserNodes(id, body.NodeIDs); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	u.NodeIDs = body.NodeIDs
	_ = s.syncXray()
	writeJSON(w, 200, u)
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, 400, "无效 id")
		return
	}
	cur, err := s.db.GetUser(id)
	if err != nil {
		writeErr(w, 404, "用户不存在")
		return
	}
	var body struct {
		Email    string  `json:"email"`
		Remark   string  `json:"remark"`
		PlanID   *int64  `json:"plan_id"`
		Enabled  *bool   `json:"enabled"`
		ExpireAt *string `json:"expire_at"`
		NodeIDs  []int64 `json:"node_ids"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	if strings.TrimSpace(body.Email) != "" {
		cur.Email = strings.TrimSpace(body.Email)
	}
	cur.Remark = body.Remark
	cur.PlanID = body.PlanID
	if body.Enabled != nil {
		cur.Enabled = *body.Enabled
	}
	if body.ExpireAt != nil {
		if *body.ExpireAt == "" {
			cur.ExpireAt = nil
		} else {
			t, err := time.Parse(time.RFC3339, *body.ExpireAt)
			if err != nil {
				t, err = time.Parse("2006-01-02", *body.ExpireAt)
			}
			if err != nil {
				writeErr(w, 400, "到期时间格式错误")
				return
			}
			cur.ExpireAt = &t
		}
	}
	if err := s.db.UpdateUser(*cur); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if body.NodeIDs != nil {
		if err := s.db.SetUserNodes(cur.ID, body.NodeIDs); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		cur.NodeIDs = body.NodeIDs
	} else {
		cur.NodeIDs, _ = s.db.UserNodeIDs(cur.ID)
	}
	_ = s.syncXray()
	writeJSON(w, 200, cur)
}

func (s *Server) resetTraffic(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, 400, "无效 id")
		return
	}
	if err := s.db.ResetUserTraffic(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) resetUUID(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, 400, "无效 id")
		return
	}
	u, err := s.db.GetUser(id)
	if err != nil {
		writeErr(w, 404, "用户不存在")
		return
	}
	u.UUID = uuid.NewString()
	tok, err := auth.RandomToken(16)
	if err == nil {
		u.SubToken = tok
	}
	if err := s.db.UpdateUser(*u); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = s.syncXray()
	writeJSON(w, 200, u)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, 400, "无效 id")
		return
	}
	if err := s.db.DeleteUser(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = s.syncXray()
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) getInbound(w http.ResponseWriter, r *http.Request) {
	in, err := s.db.GetInbound()
	if err != nil {
		writeErr(w, 404, "尚未配置入站")
		return
	}
	if err := s.repairInbound(in); err != nil {
		writeErr(w, 500, "修复 Reality 密钥失败："+err.Error())
		return
	}
	writeJSON(w, 200, s.inboundView(*in))
}

func (s *Server) inboundView(in models.Inbound) map[string]any {
	keysOK := true
	switch strings.ToLower(in.Protocol) {
	case "shadowsocks", "ss":
		keysOK = xray.ValidSS2022Password(in.Method, in.Password)
	case "vmess":
		keysOK = true
	default:
		if in.Security == "none" {
			keysOK = true
		} else {
			keysOK = xray.ValidRealityKey(in.PrivateKey) && xray.ValidRealityKey(in.PublicKey)
		}
	}
	link, host := s.inboundShareLink(in)
	return map[string]any{
		"id":           in.ID,
		"node_id":      in.NodeID,
		"node_name":    in.NodeName,
		"remark":       in.Remark,
		"tag":          in.Tag,
		"protocol":     in.Protocol,
		"listen":       in.Listen,
		"port":         in.Port,
		"flow":         in.Flow,
		"security":     in.Security,
		"dest":         in.Dest,
		"server_name":  in.ServerName,
		"private_key":  in.PrivateKey,
		"public_key":   in.PublicKey,
		"short_id":     in.ShortID,
		"method":       in.Method,
		"password":     in.Password,
		"enabled":      in.Enabled,
		"keys_ok":      keysOK,
		"share_link":   link,
		"share_host":   host,
		"xray_running": s.xray.Running(),
		"xray_path":    s.xray.Bin(),
		"xray_message": s.xray.Message(),
		"dev":          s.cfg.Dev,
		"default_port": s.defaultInboundPort(),
	}
}

func (s *Server) inboundShareLink(in models.Inbound) (string, string) {
	host := ""
	if in.NodeID > 0 {
		if n, err := s.db.GetNode(in.NodeID); err == nil {
			host = nodeconfig.NodeAddress(*n, "")
		}
	}
	if host == "" {
		host = sub.PublicHost(s.db.GetSetting("public_url", ""), "")
	}
	name := in.Remark
	if name == "" {
		name = in.Tag
	}
	ep := sub.Endpoint{
		Name:       name,
		Host:       host,
		Port:       in.Port,
		Protocol:   in.Protocol,
		Flow:       in.Flow,
		Security:   in.Security,
		ServerName: in.ServerName,
		PublicKey:  in.PublicKey,
		ShortID:    in.ShortID,
		Method:     in.Method,
		Password:   in.Password,
	}
	proto := strings.ToLower(in.Protocol)
	if proto == "shadowsocks" || proto == "ss" {
		return sub.ShareLink(ep, models.User{Email: name, Remark: name}, name), host
	}
	users, _ := s.db.ActiveUsers()
	if len(users) == 0 {
		return "", host
	}
	return sub.ShareLink(ep, users[0], name), host
}

func (s *Server) normalizeInbound(in *models.Inbound) error {
	in.Protocol = strings.ToLower(strings.TrimSpace(in.Protocol))
	switch in.Protocol {
	case "vmess", "shadowsocks", "ss":
	default:
		in.Protocol = "vless"
	}
	if in.Protocol == "ss" {
		in.Protocol = "shadowsocks"
	}
	if in.Listen == "" {
		in.Listen = "0.0.0.0"
	}
	if in.Port == 0 {
		in.Port = s.defaultInboundPort()
	}
	if in.Tag == "" {
		in.Tag = fmt.Sprintf("%s-%d", in.Protocol, in.Port)
	}
	switch in.Protocol {
	case "vmess":
		in.Security = "none"
		in.Flow = ""
	case "shadowsocks":
		in.Security = "none"
		in.Flow = ""
		method, password, err := xray.NormalizeSS(in.Method, in.Password)
		if err != nil {
			return err
		}
		in.Method = method
		in.Password = password
	default:
		if in.Security == "none" {
			in.Flow = ""
		} else {
			in.Security = "reality"
			if in.Dest == "" {
				in.Dest = "www.microsoft.com:443"
			}
			if in.ServerName == "" {
				in.ServerName = strings.Split(in.Dest, ":")[0]
			}
			if in.Flow == "" {
				in.Flow = "xtls-rprx-vision"
			}
			if xray.IsPlaceholderKey(in.PrivateKey) || xray.IsPlaceholderKey(in.PublicKey) ||
				!xray.ValidRealityKey(in.PrivateKey) || !xray.ValidRealityKey(in.PublicKey) {
				priv, pub, err := xray.GenerateRealityKeys(s.xray.Bin())
				if err != nil {
					return fmt.Errorf("生成 Reality 密钥失败：%w", err)
				}
				in.PrivateKey = priv
				in.PublicKey = pub
			}
			if strings.TrimSpace(in.ShortID) == "" {
				in.ShortID = xray.RandomShortID()
			}
		}
	}
	return s.ensureUniquePort(in)
}

func (s *Server) ensureUniquePort(in *models.Inbound) error {
	if in.NodeID <= 0 {
		return nil
	}
	items, err := s.db.ListInboundsForNode(in.NodeID)
	if err != nil {
		return err
	}
	for _, other := range items {
		if other.ID == in.ID || !other.Enabled {
			continue
		}
		listen := other.Listen
		if listen == "" {
			listen = "0.0.0.0"
		}
		mine := in.Listen
		if mine == "" {
			mine = "0.0.0.0"
		}
		if other.Port == in.Port && (listen == mine || listen == "0.0.0.0" || mine == "0.0.0.0") {
			label := other.Remark
			if label == "" {
				label = other.Tag
			}
			return fmt.Errorf("这台服务器端口 %d 已被「%s」占用，请换端口", in.Port, label)
		}
	}
	return nil
}

func (s *Server) afterInboundSave(in *models.Inbound) error {
	if in.NodeID > 0 && in.Protocol == "vless" && in.Security != "none" && in.Port > 0 {
		if n, err := s.db.GetNode(in.NodeID); err == nil {
			n.Port = in.Port
			_ = s.db.UpdateNode(*n)
		}
	}
	return s.syncXray()
}

func (s *Server) listInbounds(w http.ResponseWriter, r *http.Request) {
	items, err := s.db.ListInbounds()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	users, _ := s.db.ActiveUsers()
	out := make([]map[string]any, 0, len(items))
	for _, in := range items {
		_ = s.repairInbound(&in)
		v := s.inboundView(in)
		v["client_num"] = len(users)
		out = append(out, v)
	}
	writeJSON(w, 200, map[string]any{
		"items":        out,
		"xray_running": s.xray.Running(),
		"xray_path":    s.xray.Bin(),
		"xray_message": s.xray.Message(),
	})
}

func (s *Server) createInbound(w http.ResponseWriter, r *http.Request) {
	var in models.Inbound
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	in.ID = 0
	in.Enabled = true
	if in.NodeID <= 0 {
		writeErr(w, 400, "请先选择服务器，再在这台机器上添加协议")
		return
	}
	if _, err := s.db.GetNode(in.NodeID); err != nil {
		writeErr(w, 400, "所选服务器不存在")
		return
	}
	if err := s.normalizeInbound(&in); err != nil {
		writeInboundErr(w, err)
		return
	}
	if in.Remark == "" {
		in.Remark = fmt.Sprintf("%s-%d", strings.ToUpper(in.Protocol), in.Port)
	}
	if err := s.db.SaveInbound(&in); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := s.afterInboundSave(&in); err != nil {
		v := s.inboundView(in)
		v["ok"] = true
		v["warning"] = err.Error()
		writeJSON(w, 200, v)
		return
	}
	writeJSON(w, 200, s.inboundView(in))
}

func (s *Server) updateInbound(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, 400, "无效 id")
		return
	}
	cur, err := s.db.GetInboundByID(id)
	if err != nil {
		writeErr(w, 404, "入站不存在")
		return
	}
	var in models.Inbound
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	in.ID = cur.ID
	if in.NodeID <= 0 {
		in.NodeID = cur.NodeID
	}
	if in.NodeID <= 0 {
		writeErr(w, 400, "入站必须绑定一台服务器")
		return
	}
	if err := s.normalizeInbound(&in); err != nil {
		writeInboundErr(w, err)
		return
	}
	if err := s.db.SaveInbound(&in); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := s.afterInboundSave(&in); err != nil {
		v := s.inboundView(in)
		v["ok"] = true
		v["warning"] = err.Error()
		writeJSON(w, 200, v)
		return
	}
	writeJSON(w, 200, s.inboundView(in))
}

func (s *Server) deleteInbound(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, 400, "无效 id")
		return
	}
	all, _ := s.db.ListInbounds()
	if len(all) <= 1 {
		writeErr(w, 400, "至少保留一条入站")
		return
	}
	if err := s.db.DeleteInbound(id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	_ = s.syncXray()
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) regenInboundKeys(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, 400, "无效 id")
		return
	}
	in, err := s.db.GetInboundByID(id)
	if err != nil {
		writeErr(w, 404, "入站不存在")
		return
	}
	if strings.ToLower(in.Protocol) != "vless" || in.Security == "none" {
		writeErr(w, 400, "只有 VLESS+Reality 才需要换密钥")
		return
	}
	priv, pub, err := xray.GenerateRealityKeys(s.xray.Bin())
	if err != nil {
		writeErr(w, 500, "生成 Reality 密钥失败："+err.Error())
		return
	}
	in.PrivateKey = priv
	in.PublicKey = pub
	in.ShortID = xray.RandomShortID()
	if err := s.db.SaveInbound(in); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := s.afterInboundSave(in); err != nil {
		v := s.inboundView(*in)
		v["ok"] = true
		v["warning"] = err.Error()
		writeJSON(w, 200, v)
		return
	}
	writeJSON(w, 200, s.inboundView(*in))
}

func (s *Server) putInbound(w http.ResponseWriter, r *http.Request) {
	var in models.Inbound
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	cur, err := s.db.GetInbound()
	if err == nil {
		in.ID = cur.ID
		if in.NodeID == 0 {
			in.NodeID = cur.NodeID
		}
		if in.Remark == "" {
			in.Remark = cur.Remark
		}
		if in.Tag == "" {
			in.Tag = cur.Tag
		}
		in.Enabled = true
	}
	if err := s.normalizeInbound(&in); err != nil {
		writeInboundErr(w, err)
		return
	}
	if err := s.db.SaveInbound(&in); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := s.afterInboundSave(&in); err != nil {
		view := s.inboundView(in)
		view["ok"] = true
		view["warning"] = err.Error()
		writeJSON(w, 200, view)
		return
	}
	writeJSON(w, 200, s.inboundView(in))
}

func (s *Server) regenKeys(w http.ResponseWriter, r *http.Request) {
	in, err := s.db.GetInbound()
	if err != nil {
		writeErr(w, 404, "尚未配置入站")
		return
	}
	priv, pub, err := xray.GenerateRealityKeys(s.xray.Bin())
	if err != nil {
		writeErr(w, 500, "生成 Reality 密钥失败："+err.Error())
		return
	}
	in.PrivateKey = priv
	in.PublicKey = pub
	in.ShortID = xray.RandomShortID()
	if err := s.db.SaveInbound(in); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := s.syncXray(); err != nil {
		view := s.inboundView(*in)
		view["ok"] = true
		view["warning"] = err.Error()
		writeJSON(w, 200, view)
		return
	}
	writeJSON(w, 200, s.inboundView(*in))
}

func (s *Server) listOutbounds(w http.ResponseWriter, r *http.Request) {
	items, err := s.db.ListOutbounds()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *Server) createOutbound(w http.ResponseWriter, r *http.Request) {
	var o models.Outbound
	if err := readJSON(r, &o); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	o.ID = 0
	o.Enabled = true
	o.IsDefault = false
	if o.Protocol == "" {
		o.Protocol = "freedom"
	}
	if o.Tag == "" {
		o.Tag = o.Protocol
	}
	if o.Remark == "" {
		o.Remark = o.Tag
	}
	if err := s.db.SaveOutbound(&o); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = s.syncXray()
	writeJSON(w, 200, o)
}

func (s *Server) updateOutbound(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, 400, "无效 id")
		return
	}
	cur, err := s.db.GetOutbound(id)
	if err != nil {
		writeErr(w, 404, "出站不存在")
		return
	}
	var o models.Outbound
	if err := readJSON(r, &o); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	o.ID = cur.ID
	if cur.IsDefault {
		o.IsDefault = true
		if o.Protocol == "" {
			o.Protocol = cur.Protocol
		}
		if o.Tag == "" {
			o.Tag = cur.Tag
		}
	}
	if err := s.db.SaveOutbound(&o); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = s.syncXray()
	writeJSON(w, 200, o)
}

func (s *Server) deleteOutbound(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, 400, "无效 id")
		return
	}
	o, err := s.db.GetOutbound(id)
	if err != nil {
		writeErr(w, 404, "出站不存在")
		return
	}
	if o.IsDefault {
		writeErr(w, 400, "默认出站不能删除")
		return
	}
	if err := s.db.DeleteOutbound(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = s.syncXray()
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) reloadXray(w http.ResponseWriter, r *http.Request) {
	if err := s.syncXray(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "running": s.xray.Running(), "message": s.xray.Message()})
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, models.Settings{
		PublicURL: s.db.GetSetting("public_url", s.cfg.PublicURL),
		Listen:    s.db.GetSetting("listen", s.cfg.Listen),
		XrayPath:  s.db.GetSetting("xray_path", s.xray.Bin()),
		PanelHost: r.Host,
		Version:   s.version,
		Repo:      update.DefaultRepo,
	})
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var body models.Settings
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	_ = s.db.SetSetting("public_url", strings.TrimSpace(body.PublicURL))
	if strings.TrimSpace(body.XrayPath) != "" {
		_ = s.db.SetSetting("xray_path", strings.TrimSpace(body.XrayPath))
		s.xray.SetBin(strings.TrimSpace(body.XrayPath))
	}
	writeJSON(w, 200, body)
}

func (s *Server) subscription(w http.ResponseWriter, r *http.Request) {
	u, in, err := s.subUser(r)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	fallback := sub.PublicHost(s.db.GetSetting("public_url", ""), hostname(r))
	eps, err := nodeconfig.Endpoints(s.db, *u, *in, fallback)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Profile-Update-Interval", "24")
	_, _ = w.Write([]byte(sub.Base64Links(sub.LinksForEndpoints(eps, *u))))
}

func (s *Server) subscriptionClash(w http.ResponseWriter, r *http.Request) {
	u, in, err := s.subUser(r)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	fallback := sub.PublicHost(s.db.GetSetting("public_url", ""), hostname(r))
	eps, err := nodeconfig.Endpoints(s.db, *u, *in, fallback)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	_, _ = w.Write([]byte(sub.ClashYAMLMulti(eps, *u)))
}

func (s *Server) subUser(r *http.Request) (*models.User, *models.Inbound, error) {
	token := chi.URLParam(r, "token")
	u, err := s.db.GetUserBySubToken(token)
	if err != nil {
		return nil, nil, err
	}
	if !s.db.UserActive(*u) {
		return nil, nil, sql.ErrNoRows
	}
	ids, _ := s.db.UserNodeIDs(u.ID)
	u.NodeIDs = ids
	in, err := s.db.GetInbound()
	if err != nil {
		return nil, nil, err
	}
	return u, in, nil
}

func (s *Server) RepairInbound() error {
	in, err := s.db.GetInbound()
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if err := s.repairInbound(in); err != nil {
		return err
	}
	if err := s.ensureLocalNode(in.Port, s.db.GetSetting("public_url", "")); err != nil {
		return err
	}
	s.ensureNodeInbounds()
	return nil
}

func (s *Server) seedInboundForNode(n models.Node) error {
	existing, err := s.db.ListInboundsForNode(n.ID)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	port := n.Port
	if port == 0 {
		port = s.defaultInboundPort()
	}
	in := models.Inbound{
		NodeID:     n.ID,
		Remark:     n.Name,
		Tag:        fmt.Sprintf("in-%d", n.ID),
		Protocol:   "vless",
		Listen:     "0.0.0.0",
		Port:       port,
		Flow:       "xtls-rprx-vision",
		Security:   "reality",
		Dest:       "www.microsoft.com:443",
		ServerName: "www.microsoft.com",
		Enabled:    true,
	}
	if err := s.normalizeInbound(&in); err != nil {
		return err
	}
	return s.db.SaveInbound(&in)
}

func (s *Server) ensureNodeInbounds() {
	nodes, err := s.db.ListNodes()
	if err != nil {
		return
	}
	for _, n := range nodes {
		_ = s.seedInboundForNode(n)
	}
}

func (s *Server) repairAllInbounds() {
	items, err := s.db.ListInbounds()
	if err != nil {
		return
	}
	for i := range items {
		_ = s.repairInbound(&items[i])
	}
}

func (s *Server) ensureLocalNode(port int, publicURL string) error {
	if local, err := s.db.LocalNode(); err == nil {
		_, _ = s.db.SQL.Exec(`UPDATE inbounds SET node_id=? WHERE node_id=0`, local.ID)
		return nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if port == 0 {
		port = s.defaultInboundPort()
	}
	tok, err := auth.RandomToken(24)
	if err != nil {
		return err
	}
	host := sub.PublicHost(publicURL, "")
	id, err := s.db.CreateNode(models.Node{
		Name:       "本机",
		Token:      tok,
		PublicHost: host,
		Port:       port,
		IsLocal:    true,
		Enabled:    true,
		Subscribe:  true,
	})
	if err == nil {
		_, _ = s.db.BumpConfigRev()
		_, _ = s.db.SQL.Exec(`UPDATE inbounds SET node_id=? WHERE node_id=0`, id)
	}
	return err
}

func (s *Server) SyncXray() error { return s.syncXray() }

func (s *Server) syncXray() error {
	in, err := s.db.GetInbound()
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if in != nil {
		if err := s.repairInbound(in); err != nil {
			return err
		}
		_ = s.ensureLocalNode(in.Port, s.db.GetSetting("public_url", ""))
	} else {
		_ = s.ensureLocalNode(s.defaultInboundPort(), s.db.GetSetting("public_url", ""))
	}
	s.ensureNodeInbounds()
	s.repairAllInbounds()
	_, _ = s.db.BumpConfigRev()
	local, err := s.db.LocalNode()
	if err != nil {
		return nil
	}
	base := models.Inbound{}
	if in != nil {
		base = *in
	}
	cfg, err := nodeconfig.Build(s.db, *local, base)
	if err != nil {
		return err
	}
	if err := s.xray.WriteConfigBytes(cfg); err != nil {
		return err
	}
	if _, err := os.Stat(s.xray.Bin()); err != nil {
		if bin, e := xray.EnsureBinary(s.xray.Bin()); e == nil {
			s.xray.SetBin(bin)
			_ = s.db.SetSetting("xray_path", bin)
		} else {
			return fmt.Errorf("本机没有 Xray：%w", e)
		}
	}
	return s.xray.Reload()
}

func hostname(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		return r.Host
	}
	return host
}

func parseID(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

func writeInboundErr(w http.ResponseWriter, err error) {
	msg := err.Error()
	if strings.Contains(msg, "端口") {
		writeErr(w, 400, msg)
		return
	}
	writeErr(w, 500, msg)
}

func (s *Server) SeedInboundIfNeeded() error {
	return s.ensureInbound(0)
}

func (s *Server) repo() string {
	if v := s.db.GetSetting("release_repo", ""); v != "" {
		return v
	}
	return update.DefaultRepo
}

func (s *Server) agentDir() string {
	return filepath.Join(s.cfg.DataDir, "agents")
}

func (s *Server) stagedAgents() map[string]string {
	out := map[string]string{}
	for _, arch := range []string{"amd64", "arm64"} {
		p := filepath.Join(s.agentDir(), "hallo-agent-linux-"+arch)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			out[arch] = p
		}
	}
	return out
}

func (s *Server) updateStatus(w http.ResponseWriter, r *http.Request) {
	rel, err := update.FetchLatest(s.repo())
	if err != nil {
		writeErr(w, 502, "拉取 GitHub 失败："+err.Error())
		return
	}
	asset := update.AssetName("panel", "linux", update.Arch())
	st := update.Status{
		Current:     s.version,
		Latest:      rel.Tag,
		Newer:       update.Newer(rel.Tag, s.version),
		HTMLURL:     rel.HTMLURL,
		Repo:        s.repo(),
		Arch:        update.Arch(),
		Asset:       asset,
		AgentStaged: s.stagedAgents(),
	}
	writeJSON(w, 200, st)
}

func (s *Server) applyPanelUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.upMu.TryLock() {
		writeErr(w, 409, "已有更新在进行")
		return
	}
	defer s.upMu.Unlock()

	rel, err := update.FetchLatest(s.repo())
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	if !update.Newer(rel.Tag, s.version) {
		writeJSON(w, 200, map[string]any{"ok": true, "message": "已是最新 " + s.version, "latest": rel.Tag})
		return
	}
	assetName := update.AssetName("panel", "linux", update.Arch())
	asset, err := update.FindAsset(rel, assetName)
	if err != nil {
		writeErr(w, 400, err.Error()+"。当前架构需要 Linux 发行包，macOS 开发机请用 git pull / 重新编译。")
		return
	}
	dir, err := os.MkdirTemp("", "hallo-up-*")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer os.RemoveAll(dir)
	tgz := filepath.Join(dir, assetName)
	if err := update.Download(asset.URL, tgz); err != nil {
		writeErr(w, 502, "下载失败："+err.Error())
		return
	}
	bin := filepath.Join(dir, "hallo")
	if err := update.ExtractFile(tgz, "hallo", bin); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	self, err := update.SelfPath()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := update.ReplaceExec(bin, self); err != nil {
		writeErr(w, 500, "写入二进制失败："+err.Error())
		return
	}
	if err := s.stageAgentsFromRelease(rel); err != nil {
		writeJSON(w, 200, map[string]any{
			"ok":      true,
			"version": rel.Tag,
			"warning": "面板已更新，暂存 agent 失败：" + err.Error(),
		})
	} else {
		writeJSON(w, 200, map[string]any{"ok": true, "version": rel.Tag, "restarting": true})
	}
	go func() {
		time.Sleep(800 * time.Millisecond)
		if err := update.RestartPanel(); err != nil {
			os.Exit(0)
		}
	}()
}

func (s *Server) stageAgentsFromRelease(rel *update.Release) error {
	dir := s.agentDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "hallo-agent-stage-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	var last error
	ok := 0
	for _, arch := range []string{"amd64", "arm64"} {
		name := update.AssetName("agent", "linux", arch)
		asset, err := update.FindAsset(rel, name)
		if err != nil {
			last = err
			continue
		}
		tgz := filepath.Join(tmp, name)
		if err := update.Download(asset.URL, tgz); err != nil {
			last = err
			continue
		}
		dest := filepath.Join(dir, "hallo-agent-linux-"+arch)
		if err := update.ExtractFile(tgz, "hallo-agent", dest); err != nil {
			last = err
			continue
		}
		ok++
	}
	if ok == 0 {
		if last != nil {
			return last
		}
		return errors.New("没有可用的 agent 附件")
	}
	return nil
}

func (s *Server) listNodes(w http.ResponseWriter, r *http.Request) {
	items, err := s.db.ListNodes()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for i := range items {
		if items[i].IsLocal {
			items[i].Online = true
			items[i].XrayRunning = s.xray.Running()
			items[i].XrayMessage = s.xray.Message()
			if items[i].PublicHost == "" {
				items[i].PublicHost = sub.PublicHost(s.db.GetSetting("public_url", ""), hostname(r))
			}
		}
	}
	writeJSON(w, 200, map[string]any{"items": items, "panel_version": s.version, "agent_staged": s.stagedAgents()})
}

func (s *Server) createNode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		PublicHost  string `json:"public_host"`
		Port        int    `json:"port"`
		RelayNodeID *int64 `json:"relay_node_id"`
		Subscribe   *bool  `json:"subscribe"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		writeErr(w, 400, "节点名不能为空")
		return
	}
	tok, err := auth.RandomToken(24)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	port := body.Port
	if port == 0 {
		if in, err := s.db.GetInbound(); err == nil {
			port = in.Port
		}
		if port == 0 {
			port = s.defaultInboundPort()
		}
	}
	n := models.Node{
		Name:        body.Name,
		Token:       tok,
		PublicHost:  strings.TrimSpace(body.PublicHost),
		Port:        port,
		RelayNodeID: body.RelayNodeID,
		Enabled:     true,
		Subscribe:   true,
	}
	if body.Enabled != nil {
		n.Enabled = *body.Enabled
	}
	if body.Subscribe != nil {
		n.Subscribe = *body.Subscribe
	}
	if n.RelayNodeID != nil && *n.RelayNodeID > 0 {
		uuidTok, err := auth.RandomToken(16)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		n.RelayUUID = formatRelayUUID(uuidTok)
	}
	id, err := s.db.CreateNode(n)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	saved, err := s.db.GetNode(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := s.seedInboundForNode(*saved); err != nil {
		writeErr(w, 500, "节点已登记，但创建入站失败："+err.Error())
		return
	}
	_ = s.syncXray()
	pub := s.publicBase(r)
	install := "curl -fsSL '" + pub + "/install/agent.sh?token=" + saved.Token + "' | sh"
	writeJSON(w, 200, map[string]any{
		"node":         saved,
		"install_hint": install,
		"panel":        pub,
		"message":      "服务器已登记。把安装命令拿到那台机器用 root 执行后，再在「入站」里选这台机器添加协议。",
	})
}

func formatRelayUUID(hexTok string) string {
	h := strings.ReplaceAll(hexTok, "-", "")
	if len(h) < 32 {
		h = h + strings.Repeat("0", 32-len(h))
	}
	h = h[:32]
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func (s *Server) updateNode(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, 400, "无效 id")
		return
	}
	cur, err := s.db.GetNode(id)
	if err != nil {
		writeErr(w, 404, "节点不存在")
		return
	}
	var body struct {
		Name        string `json:"name"`
		PublicHost  string `json:"public_host"`
		Port        *int   `json:"port"`
		RelayNodeID *int64 `json:"relay_node_id"`
		Subscribe   *bool  `json:"subscribe"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	if strings.TrimSpace(body.Name) != "" {
		cur.Name = strings.TrimSpace(body.Name)
	}
	if body.PublicHost != "" {
		cur.PublicHost = strings.TrimSpace(body.PublicHost)
	}
	if body.Port != nil && *body.Port > 0 {
		cur.Port = *body.Port
	}
	if body.Subscribe != nil {
		cur.Subscribe = *body.Subscribe
	}
	if body.Enabled != nil {
		cur.Enabled = *body.Enabled
	}
	if body.RelayNodeID != nil {
		if *body.RelayNodeID <= 0 {
			cur.RelayNodeID = nil
			cur.RelayUUID = ""
		} else if cur.RelayNodeID == nil || *cur.RelayNodeID != *body.RelayNodeID {
			idv := *body.RelayNodeID
			cur.RelayNodeID = &idv
			tok, err := auth.RandomToken(16)
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			cur.RelayUUID = formatRelayUUID(tok)
		}
	}
	if err := s.db.UpdateNode(*cur); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = s.syncXray()
	writeJSON(w, 200, cur)
}

func (s *Server) deleteNode(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, 400, "无效 id")
		return
	}
	if err := s.db.DeleteNode(id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	_ = s.syncXray()
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) pushNodeUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, 400, "无效 id")
		return
	}
	n, err := s.db.GetNode(id)
	if err != nil {
		writeErr(w, 404, "节点不存在")
		return
	}
	ver := s.desiredAgentVersion()
	if err := s.db.SetNodeForce(n.ID, ver, true); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "desired_version": ver, "message": "已标记推送，节点下次心跳会从面板拉包"})
}

func (s *Server) pushAllNodeUpdates(w http.ResponseWriter, r *http.Request) {
	ver := s.desiredAgentVersion()
	if err := s.db.SetAllNodesForce(ver); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "desired_version": ver})
}

func (s *Server) desiredAgentVersion() string {
	if s.version != "" && s.version != "dev" {
		return s.version
	}
	return "latest"
}

func (s *Server) agentHeartbeat(w http.ResponseWriter, r *http.Request) {
	var hb agentproto.Heartbeat
	if err := readJSON(r, &hb); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	n, err := s.db.GetNodeByToken(strings.TrimSpace(hb.Token))
	if err != nil {
		writeErr(w, 401, "无效 token")
		return
	}
	host := strings.TrimSpace(hb.Host)
	if strings.TrimSpace(hb.PublicIP) != "" {
		host = strings.TrimSpace(hb.PublicIP)
	}
	_ = s.db.TouchNode(n.ID, hb.Version, hb.Arch, host, hb.XrayRunning, hb.XrayMessage)
	if host != "" {
		ipLike := net.ParseIP(host) != nil
		curIP := net.ParseIP(n.PublicHost)
		if n.PublicHost == "" || (ipLike && curIP == nil && n.PublicHost != host) {
			n.PublicHost = host
			_ = s.db.UpdateNode(*n)
		}
	}
	if n.ForceUpdate && hb.Version != "" && n.DesiredVer != "" && n.DesiredVer != "latest" && !update.Newer(n.DesiredVer, hb.Version) {
		_ = s.db.ClearNodeForce(n.ID)
		n.ForceUpdate = false
	}
	pub := s.publicBase(r)
	arch := hb.Arch
	if arch == "" {
		arch = n.Arch
	}
	if arch == "x86_64" {
		arch = "amd64"
	}
	if arch == "aarch64" {
		arch = "arm64"
	}
	fresh, _ := s.db.GetNode(n.ID)
	if fresh != nil {
		n = fresh
	}
	reply := agentproto.HeartbeatReply{
		OK:           true,
		PanelVersion: s.version,
		DesiredVer:   n.DesiredVer,
		ForceUpdate:  n.ForceUpdate,
		ConfigRev:    n.ConfigRev,
		Node: agentproto.NodeInfo{
			ID:         n.ID,
			Name:       n.Name,
			PublicHost: n.PublicHost,
			Port:       n.Port,
			Enabled:    n.Enabled,
		},
	}
	if n.ForceUpdate && pub != "" && arch != "" {
		reply.UpdateURL = pub + "/download/agent/" + arch
	}
	needCfg := n.Enabled && (!hb.XrayRunning || hb.ConfigRev == "" || hb.ConfigRev != n.ConfigRev)
	if needCfg {
		s.ensureNodeInbounds()
		s.repairAllInbounds()
		if n.ConfigRev == "" {
			if rev, e := s.db.BumpConfigRev(); e == nil {
				n.ConfigRev = rev
			}
		}
		base := models.Inbound{}
		if in, err := s.db.GetInbound(); err == nil && in != nil {
			base = *in
		}
		if cfg, err := nodeconfig.Build(s.db, *n, base); err == nil {
			reply.Config = cfg
			reply.ConfigRev = n.ConfigRev
		}
	}
	writeJSON(w, 200, reply)
}

func (s *Server) downloadAgent(w http.ResponseWriter, r *http.Request) {
	arch := chi.URLParam(r, "arch")
	switch arch {
	case "x86_64":
		arch = "amd64"
	case "aarch64":
		arch = "arm64"
	}
	if arch != "amd64" && arch != "arm64" {
		http.Error(w, "unsupported arch", http.StatusNotFound)
		return
	}
	p := filepath.Join(s.agentDir(), "hallo-agent-linux-"+arch)
	if _, err := os.Stat(p); err != nil {
		http.Error(w, "agent 尚未暂存，请先在设置里点更新，或把 hallo-agent 放到 "+p, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="hallo-agent"`)
	http.ServeFile(w, r, p)
}

func (s *Server) agentInstallScript(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}
	n, err := s.db.GetNodeByToken(token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	pub := s.publicBase(r)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, `#!/bin/sh
set -eu
if [ "$(id -u)" != "0" ]; then
  echo "请用 root 执行：curl -fsSL '%s/install/agent.sh?token=%s' | sh" >&2
  exit 1
fi
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "不支持的架构：$arch" >&2; exit 1 ;;
esac
url="%s/download/agent/$arch"
echo "下载 hallo-agent $url"
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
curl -fL --connect-timeout 15 --max-time 180 -o "$tmp" "$url"
# 简单校验 ELF，避免把 HTML 错误页当成二进制
magic=$(od -An -N4 -tx1 "$tmp" | tr -d ' \n')
if [ "$magic" != "7f454c46" ]; then
  echo "下载到的不是 Linux 可执行文件（可能面板上还没暂存 agent）" >&2
  head -c 200 "$tmp" >&2 || true
  exit 1
fi
install -m 0755 "$tmp" /usr/local/bin/hallo-agent
/usr/local/bin/hallo-agent install --panel '%s' --node-id '%d' --token '%s'
`, pub, token, pub, pub, n.ID, token)
}

func (s *Server) publicBase(r *http.Request) string {
	pub := strings.TrimRight(s.db.GetSetting("public_url", ""), "/")
	if pub != "" {
		return pub
	}
	scheme := "http"
	if s.cookieSecure(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
