package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"hallo/internal/auth"
	"hallo/internal/config"
	"hallo/internal/db"
	"hallo/internal/models"
	"hallo/internal/sub"
	"hallo/internal/xray"
)

type Server struct {
	cfg  config.Config
	db   *db.DB
	xray *xray.Manager
	web  fs.FS
}

func New(cfg config.Config, database *db.DB, xm *xray.Manager, webFS fs.FS) *Server {
	return &Server{cfg: cfg, db: database, xray: xm, web: webFS}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Get("/api/meta", s.meta)
	r.Post("/api/setup", s.setup)
	r.Post("/api/login", s.login)
	r.Post("/api/logout", s.logout)

	r.Get("/sub/{token}", s.subscription)
	r.Get("/sub/{token}/clash", s.subscriptionClash)

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
		r.Post("/api/xray/reload", s.reloadXray)
		r.Get("/api/settings", s.getSettings)
		r.Put("/api/settings", s.putSettings)
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

func (s *Server) ensureInbound(port int) error {
	if _, err := s.db.GetInbound(); err == nil {
		return nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if port == 0 {
		port = 443
		if s.cfg.Dev {
			port = 18443
		}
	}
	priv, pub, err := xray.GenerateRealityKeys(s.xray.Bin())
	if err != nil {
		priv, pub = "CHANGE_ME_PRIVATE", "CHANGE_ME_PUBLIC"
	}
	return s.db.SaveInbound(models.Inbound{
		Protocol:   "vless",
		Listen:     "0.0.0.0",
		Port:       port,
		Flow:       "xtls-rprx-vision",
		Dest:       "www.microsoft.com:443",
		ServerName: "www.microsoft.com",
		PrivateKey: priv,
		PublicKey:  pub,
		ShortID:    xray.RandomShortID(),
	})
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
	writeJSON(w, 200, models.Dashboard{
		SetupNeeded:  n == 0,
		UserTotal:    ut,
		UserEnabled:  ue,
		PlanTotal:    pt,
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
	host := sub.PublicHost(s.db.GetSetting("public_url", ""), hostname(r))
	in, _ := s.db.GetInbound()
	type item struct {
		models.User
		SubURL     string `json:"sub_url"`
		VlessLink  string `json:"vless_link"`
		ClashURL   string `json:"clash_url"`
		PublicHost string `json:"public_host"`
	}
	out := make([]item, 0, len(items))
	pub := strings.TrimRight(s.db.GetSetting("public_url", ""), "/")
	if pub == "" {
		scheme := "http"
		if s.cookieSecure(r) {
			scheme = "https"
		}
		pub = scheme + "://" + r.Host
	}
	for _, u := range items {
		it := item{User: u, PublicHost: host, SubURL: sub.SubURL(pub, u.SubToken), ClashURL: sub.SubURL(pub, u.SubToken) + "/clash"}
		if in != nil {
			it.VlessLink = sub.VLESSLink(host, *in, u)
		}
		out = append(out, it)
	}
	writeJSON(w, 200, map[string]any{"items": out})
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email  string `json:"email"`
		Remark string `json:"remark"`
		PlanID *int64 `json:"plan_id"`
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
	writeJSON(w, 200, in)
}

func (s *Server) putInbound(w http.ResponseWriter, r *http.Request) {
	var in models.Inbound
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	if in.Protocol == "" {
		in.Protocol = "vless"
	}
	if in.Listen == "" {
		in.Listen = "0.0.0.0"
	}
	if in.Port == 0 {
		in.Port = 443
	}
	if err := s.db.SaveInbound(in); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := s.syncXray(); err != nil {
		writeJSON(w, 200, map[string]any{"ok": true, "warning": err.Error()})
		return
	}
	writeJSON(w, 200, in)
}

func (s *Server) regenKeys(w http.ResponseWriter, r *http.Request) {
	in, err := s.db.GetInbound()
	if err != nil {
		writeErr(w, 404, "尚未配置入站")
		return
	}
	priv, pub, err := xray.GenerateRealityKeys(s.xray.Bin())
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.PrivateKey = priv
	in.PublicKey = pub
	in.ShortID = xray.RandomShortID()
	if err := s.db.SaveInbound(*in); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = s.syncXray()
	writeJSON(w, 200, in)
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
		PublicURL: s.db.GetSetting("public_url", ""),
		Listen:    s.db.GetSetting("listen", s.cfg.Listen),
		XrayPath:  s.db.GetSetting("xray_path", s.xray.Bin()),
		PanelHost: r.Host,
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
	host := sub.PublicHost(s.db.GetSetting("public_url", ""), hostname(r))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Profile-Update-Interval", "24")
	_, _ = w.Write([]byte(sub.Base64VLESS(host, *in, *u)))
}

func (s *Server) subscriptionClash(w http.ResponseWriter, r *http.Request) {
	u, in, err := s.subUser(r)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	host := sub.PublicHost(s.db.GetSetting("public_url", ""), hostname(r))
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	_, _ = w.Write([]byte(sub.ClashYAML(host, *in, *u)))
}

func (s *Server) subUser(r *http.Request) (*models.User, *models.Inbound, error) {
	token := chi.URLParam(r, "token")
	u, err := s.db.GetUserBySubToken(token)
	if err != nil {
		return nil, nil, err
	}
	if !u.Enabled {
		return nil, nil, sql.ErrNoRows
	}
	in, err := s.db.GetInbound()
	if err != nil {
		return nil, nil, err
	}
	return u, in, nil
}

func (s *Server) syncXray() error {
	in, err := s.db.GetInbound()
	if err != nil {
		return err
	}
	users, err := s.db.ActiveUsers()
	if err != nil {
		return err
	}
	if err := s.xray.WriteConfig(*in, users); err != nil {
		return err
	}
	if _, err := os.Stat(s.xray.Bin()); err != nil {
		return nil // config written; binary optional in phase 1
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

func (s *Server) SeedInboundIfNeeded() error {
	return s.ensureInbound(0)
}
