package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hallo/internal/models"

	_ "modernc.org/sqlite"
)

type DB struct {
	SQL *sql.DB
}

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	sqlDB, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	d := &DB{SQL: sqlDB}
	if err := d.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) Close() error {
	return d.SQL.Close()
}

func (d *DB) migrate() error {
	_, err := d.SQL.Exec(`
CREATE TABLE IF NOT EXISTS admins (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS sessions (
  token TEXT PRIMARY KEY,
  admin_id INTEGER NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
  expires_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS plans (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  traffic_limit INTEGER NOT NULL DEFAULT 0,
  duration_days INTEGER NOT NULL DEFAULT 0,
  note TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  email TEXT NOT NULL UNIQUE,
  remark TEXT NOT NULL DEFAULT '',
  uuid TEXT NOT NULL UNIQUE,
  plan_id INTEGER REFERENCES plans(id) ON DELETE SET NULL,
  expire_at DATETIME,
  traffic_up INTEGER NOT NULL DEFAULT 0,
  traffic_down INTEGER NOT NULL DEFAULT 0,
  enabled INTEGER NOT NULL DEFAULT 1,
  sub_token TEXT NOT NULL UNIQUE,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS inbounds (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  node_id INTEGER NOT NULL DEFAULT 0,
  remark TEXT NOT NULL DEFAULT '',
  tag TEXT NOT NULL DEFAULT 'vless-in',
  protocol TEXT NOT NULL DEFAULT 'vless',
  listen TEXT NOT NULL DEFAULT '0.0.0.0',
  port INTEGER NOT NULL DEFAULT 443,
  flow TEXT NOT NULL DEFAULT 'xtls-rprx-vision',
  dest TEXT NOT NULL,
  server_name TEXT NOT NULL,
  private_key TEXT NOT NULL,
  public_key TEXT NOT NULL,
  short_id TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS outbounds (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  node_id INTEGER NOT NULL DEFAULT 0,
  remark TEXT NOT NULL DEFAULT '',
  tag TEXT NOT NULL,
  protocol TEXT NOT NULL DEFAULT 'freedom',
  address TEXT NOT NULL DEFAULT '',
  port INTEGER NOT NULL DEFAULT 0,
  uuid TEXT NOT NULL DEFAULT '',
  flow TEXT NOT NULL DEFAULT '',
  public_key TEXT NOT NULL DEFAULT '',
  short_id TEXT NOT NULL DEFAULT '',
  server_name TEXT NOT NULL DEFAULT '',
  username TEXT NOT NULL DEFAULT '',
  password TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  is_default INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS nodes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  token TEXT NOT NULL UNIQUE,
  arch TEXT NOT NULL DEFAULT '',
  host TEXT NOT NULL DEFAULT '',
  version TEXT NOT NULL DEFAULT '',
  desired_version TEXT NOT NULL DEFAULT '',
  force_update INTEGER NOT NULL DEFAULT 0,
  last_seen DATETIME,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS user_nodes (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  node_id INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  PRIMARY KEY (user_id, node_id)
);
`)
	if err != nil {
		return err
	}
	if err := d.ensureColumns(); err != nil {
		return err
	}
	return d.EnsureBuiltinOutbounds()
}

func (d *DB) ensureColumns() error {
	type col struct{ table, name, def string }
	cols := []col{
		{"nodes", "public_host", "TEXT NOT NULL DEFAULT ''"},
		{"nodes", "port", "INTEGER NOT NULL DEFAULT 443"},
		{"nodes", "relay_node_id", "INTEGER"},
		{"nodes", "relay_uuid", "TEXT NOT NULL DEFAULT ''"},
		{"nodes", "is_local", "INTEGER NOT NULL DEFAULT 0"},
		{"nodes", "enabled", "INTEGER NOT NULL DEFAULT 1"},
		{"nodes", "subscribe", "INTEGER NOT NULL DEFAULT 1"},
		{"nodes", "xray_running", "INTEGER NOT NULL DEFAULT 0"},
		{"nodes", "xray_message", "TEXT NOT NULL DEFAULT ''"},
		{"nodes", "config_rev", "TEXT NOT NULL DEFAULT ''"},
		{"inbounds", "node_id", "INTEGER NOT NULL DEFAULT 0"},
		{"inbounds", "remark", "TEXT NOT NULL DEFAULT ''"},
		{"inbounds", "tag", "TEXT NOT NULL DEFAULT 'vless-in'"},
		{"inbounds", "enabled", "INTEGER NOT NULL DEFAULT 1"},
		{"inbounds", "security", "TEXT NOT NULL DEFAULT 'reality'"},
		{"inbounds", "method", "TEXT NOT NULL DEFAULT ''"},
		{"inbounds", "password", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, c := range cols {
		if d.hasColumn(c.table, c.name) {
			continue
		}
		if _, err := d.SQL.Exec("ALTER TABLE " + c.table + " ADD COLUMN " + c.name + " " + c.def); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) hasColumn(table, name string) bool {
	rows, err := d.SQL.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var col, typ string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &col, &typ, &notnull, &dflt, &pk); err != nil {
			return false
		}
		if col == name {
			return true
		}
	}
	return false
}

func (d *DB) AdminCount() (int, error) {
	var n int
	err := d.SQL.QueryRow(`SELECT COUNT(*) FROM admins`).Scan(&n)
	return n, err
}

func (d *DB) CreateAdmin(username, passwordHash string) error {
	_, err := d.SQL.Exec(`INSERT INTO admins (username, password_hash) VALUES (?, ?)`, username, passwordHash)
	return err
}

func (d *DB) GetAdminByUsername(username string) (*models.Admin, error) {
	row := d.SQL.QueryRow(`SELECT id, username, password_hash, created_at FROM admins WHERE username = ?`, username)
	a := &models.Admin{}
	if err := row.Scan(&a.ID, &a.Username, &a.PasswordHash, &a.CreatedAt); err != nil {
		return nil, err
	}
	return a, nil
}

func (d *DB) CreateSession(token string, adminID int64, expires time.Time) error {
	_, err := d.SQL.Exec(`INSERT INTO sessions (token, admin_id, expires_at) VALUES (?, ?, ?)`, token, adminID, expires.UTC().Format(time.RFC3339))
	return err
}

func (d *DB) DeleteSession(token string) error {
	_, err := d.SQL.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}

func (d *DB) SessionAdmin(token string) (*models.Admin, error) {
	row := d.SQL.QueryRow(`
SELECT a.id, a.username, a.password_hash, a.created_at
FROM sessions s JOIN admins a ON a.id = s.admin_id
WHERE s.token = ? AND s.expires_at > ?
`, token, time.Now().UTC().Format(time.RFC3339))
	a := &models.Admin{}
	if err := row.Scan(&a.ID, &a.Username, &a.PasswordHash, &a.CreatedAt); err != nil {
		return nil, err
	}
	return a, nil
}

func (d *DB) GetSetting(key, fallback string) string {
	var v string
	err := d.SQL.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err != nil {
		return fallback
	}
	return v
}

func (d *DB) SetSetting(key, value string) error {
	_, err := d.SQL.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (d *DB) ListPlans() ([]models.Plan, error) {
	rows, err := d.SQL.Query(`SELECT id, name, traffic_limit, duration_days, note, created_at FROM plans ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Plan
	for rows.Next() {
		var p models.Plan
		if err := rows.Scan(&p.ID, &p.Name, &p.TrafficLimit, &p.DurationDays, &p.Note, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if out == nil {
		out = []models.Plan{}
	}
	return out, rows.Err()
}

func (d *DB) GetPlan(id int64) (*models.Plan, error) {
	p := &models.Plan{}
	err := d.SQL.QueryRow(`SELECT id, name, traffic_limit, duration_days, note, created_at FROM plans WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.TrafficLimit, &p.DurationDays, &p.Note, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (d *DB) GetPlanByName(name string) (*models.Plan, error) {
	p := &models.Plan{}
	err := d.SQL.QueryRow(`SELECT id, name, traffic_limit, duration_days, note, created_at FROM plans WHERE name = ?`, name).
		Scan(&p.ID, &p.Name, &p.TrafficLimit, &p.DurationDays, &p.Note, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (d *DB) CreatePlan(p models.Plan) (int64, error) {
	res, err := d.SQL.Exec(`INSERT INTO plans (name, traffic_limit, duration_days, note) VALUES (?, ?, ?, ?)`,
		p.Name, p.TrafficLimit, p.DurationDays, p.Note)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) UpdatePlan(p models.Plan) error {
	_, err := d.SQL.Exec(`UPDATE plans SET name=?, traffic_limit=?, duration_days=?, note=? WHERE id=?`,
		p.Name, p.TrafficLimit, p.DurationDays, p.Note, p.ID)
	return err
}

func (d *DB) DeletePlan(id int64) error {
	_, err := d.SQL.Exec(`DELETE FROM plans WHERE id = ?`, id)
	return err
}

func (d *DB) ListUsers() ([]models.User, error) {
	rows, err := d.SQL.Query(`
SELECT u.id, u.email, u.remark, u.uuid, u.plan_id, COALESCE(p.name, ''), u.expire_at,
       u.traffic_up, u.traffic_down, u.enabled, u.sub_token, u.created_at
FROM users u LEFT JOIN plans p ON p.id = u.plan_id
ORDER BY u.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	if out == nil {
		out = []models.User{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, d.AttachUserNodes(out)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(s scanner) (models.User, error) {
	var u models.User
	var planID sql.NullInt64
	var expire sql.NullString
	var enabled int
	err := s.Scan(&u.ID, &u.Email, &u.Remark, &u.UUID, &planID, &u.PlanName, &expire,
		&u.TrafficUp, &u.TrafficDown, &enabled, &u.SubToken, &u.CreatedAt)
	if err != nil {
		return u, err
	}
	if planID.Valid {
		u.PlanID = &planID.Int64
	}
	if expire.Valid && expire.String != "" {
		t, err := parseTime(expire.String)
		if err == nil {
			u.ExpireAt = &t
		}
	}
	u.Enabled = enabled != 0
	return u, nil
}

func parseTime(v string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", time.RFC3339Nano} {
		if t, err := time.Parse(layout, v); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("bad time %q", v)
}

func (d *DB) GetUser(id int64) (*models.User, error) {
	row := d.SQL.QueryRow(`
	SELECT u.id, u.email, u.remark, u.uuid, u.plan_id, COALESCE(p.name, ''), u.expire_at,
	       u.traffic_up, u.traffic_down, u.enabled, u.sub_token, u.created_at
	FROM users u LEFT JOIN plans p ON p.id = u.plan_id WHERE u.id = ?`, id)
	u, err := scanUser(row)
	if err != nil {
		return nil, err
	}
	u.NodeIDs, _ = d.UserNodeIDs(u.ID)
	return &u, nil
}

func (d *DB) GetUserBySubToken(token string) (*models.User, error) {
	row := d.SQL.QueryRow(`
	SELECT u.id, u.email, u.remark, u.uuid, u.plan_id, COALESCE(p.name, ''), u.expire_at,
	       u.traffic_up, u.traffic_down, u.enabled, u.sub_token, u.created_at
	FROM users u LEFT JOIN plans p ON p.id = u.plan_id WHERE u.sub_token = ?`, token)
	u, err := scanUser(row)
	if err != nil {
		return nil, err
	}
	u.NodeIDs, _ = d.UserNodeIDs(u.ID)
	return &u, nil
}

func (d *DB) CreateUser(u models.User) (int64, error) {
	var plan any
	if u.PlanID != nil {
		plan = *u.PlanID
	}
	var expire any
	if u.ExpireAt != nil {
		expire = u.ExpireAt.UTC().Format(time.RFC3339)
	}
	enabled := 0
	if u.Enabled {
		enabled = 1
	}
	res, err := d.SQL.Exec(`
INSERT INTO users (email, remark, uuid, plan_id, expire_at, traffic_up, traffic_down, enabled, sub_token)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.Email, u.Remark, u.UUID, plan, expire, u.TrafficUp, u.TrafficDown, enabled, u.SubToken)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) UpdateUser(u models.User) error {
	var plan any
	if u.PlanID != nil {
		plan = *u.PlanID
	}
	var expire any
	if u.ExpireAt != nil {
		expire = u.ExpireAt.UTC().Format(time.RFC3339)
	}
	enabled := 0
	if u.Enabled {
		enabled = 1
	}
	_, err := d.SQL.Exec(`
UPDATE users SET email=?, remark=?, uuid=?, plan_id=?, expire_at=?, enabled=?, sub_token=? WHERE id=?`,
		u.Email, u.Remark, u.UUID, plan, expire, enabled, u.SubToken, u.ID)
	return err
}

func (d *DB) ResetUserTraffic(id int64) error {
	_, err := d.SQL.Exec(`UPDATE users SET traffic_up=0, traffic_down=0 WHERE id=?`, id)
	return err
}

func (d *DB) DeleteUser(id int64) error {
	_, err := d.SQL.Exec(`DELETE FROM users WHERE id=?`, id)
	return err
}

func (d *DB) ActiveUsers() ([]models.User, error) {
	users, err := d.ListUsers()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var out []models.User
	for _, u := range users {
		if !u.Enabled {
			continue
		}
		if u.ExpireAt != nil && now.After(*u.ExpireAt) {
			continue
		}
		if u.PlanID != nil {
			p, err := d.GetPlan(*u.PlanID)
			if err == nil && p.TrafficLimit > 0 && u.TrafficTotal() >= p.TrafficLimit {
				continue
			}
		}
		out = append(out, u)
	}
	if out == nil {
		out = []models.User{}
	}
	return out, nil
}

func (d *DB) UserActive(u models.User) bool {
	if !u.Enabled {
		return false
	}
	if u.ExpireAt != nil && time.Now().After(*u.ExpireAt) {
		return false
	}
	if u.PlanID != nil {
		p, err := d.GetPlan(*u.PlanID)
		if err == nil && p.TrafficLimit > 0 && u.TrafficTotal() >= p.TrafficLimit {
			return false
		}
	}
	return true
}

func (d *DB) SetUserNodes(userID int64, nodeIDs []int64) error {
	if _, err := d.SQL.Exec(`DELETE FROM user_nodes WHERE user_id=?`, userID); err != nil {
		return err
	}
	for _, nid := range nodeIDs {
		if nid <= 0 {
			continue
		}
		if _, err := d.SQL.Exec(`INSERT OR IGNORE INTO user_nodes (user_id, node_id) VALUES (?, ?)`, userID, nid); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) UserNodeIDs(userID int64) ([]int64, error) {
	rows, err := d.SQL.Query(`SELECT node_id FROM user_nodes WHERE user_id=? ORDER BY node_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	if out == nil {
		out = []int64{}
	}
	return out, rows.Err()
}

func (d *DB) AttachUserNodes(users []models.User) error {
	for i := range users {
		ids, err := d.UserNodeIDs(users[i].ID)
		if err != nil {
			return err
		}
		users[i].NodeIDs = ids
	}
	return nil
}

func (d *DB) UsersForNode(nodeID int64) ([]models.User, error) {
	users, err := d.ActiveUsers()
	if err != nil {
		return nil, err
	}
	if err := d.AttachUserNodes(users); err != nil {
		return nil, err
	}
	var out []models.User
	assigned := 0
	for _, u := range users {
		if len(u.NodeIDs) == 0 {
			out = append(out, u)
			continue
		}
		assigned++
		for _, id := range u.NodeIDs {
			if id == nodeID {
				out = append(out, u)
				break
			}
		}
	}
	if assigned == 0 {
		return users, nil
	}
	if out == nil {
		out = []models.User{}
	}
	return out, nil
}

const inboundSelect = `SELECT i.id, i.node_id, i.remark, i.tag, i.protocol, i.listen, i.port, i.flow, i.dest, i.server_name, i.private_key, i.public_key, i.short_id, i.enabled, COALESCE(i.security,''), COALESCE(i.method,''), COALESCE(i.password,''), COALESCE(n.name, '') FROM inbounds i LEFT JOIN nodes n ON n.id = i.node_id`

func scanInbound(s scanner) (models.Inbound, error) {
	var in models.Inbound
	var enabled int
	err := s.Scan(&in.ID, &in.NodeID, &in.Remark, &in.Tag, &in.Protocol, &in.Listen, &in.Port, &in.Flow, &in.Dest, &in.ServerName, &in.PrivateKey, &in.PublicKey, &in.ShortID, &enabled, &in.Security, &in.Method, &in.Password, &in.NodeName)
	if err != nil {
		return in, err
	}
	in.Enabled = enabled != 0
	if in.Tag == "" {
		in.Tag = "vless-in"
	}
	if in.Protocol == "" {
		in.Protocol = "vless"
	}
	if in.Security == "" && in.Protocol == "vless" {
		in.Security = "reality"
	}
	return in, nil
}

func (d *DB) GetInbound() (*models.Inbound, error) {
	row := d.SQL.QueryRow(inboundSelect + ` ORDER BY i.id LIMIT 1`)
	in, err := scanInbound(row)
	if err != nil {
		return nil, err
	}
	return &in, nil
}

func (d *DB) GetInboundByID(id int64) (*models.Inbound, error) {
	row := d.SQL.QueryRow(inboundSelect+` WHERE i.id = ?`, id)
	in, err := scanInbound(row)
	if err != nil {
		return nil, err
	}
	return &in, nil
}

func (d *DB) ListInbounds() ([]models.Inbound, error) {
	rows, err := d.SQL.Query(inboundSelect + ` ORDER BY i.node_id, i.port, i.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Inbound
	for rows.Next() {
		in, err := scanInbound(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	if out == nil {
		out = []models.Inbound{}
	}
	return out, rows.Err()
}

func (d *DB) ListInboundsForNode(nodeID int64) ([]models.Inbound, error) {
	all, err := d.ListInbounds()
	if err != nil {
		return nil, err
	}
	var out []models.Inbound
	for _, in := range all {
		if in.NodeID == nodeID {
			out = append(out, in)
		}
	}
	if out == nil {
		out = []models.Inbound{}
	}
	return out, nil
}

func (d *DB) SaveInbound(in *models.Inbound) error {
	if in.Protocol == "" {
		in.Protocol = "vless"
	}
	if in.Listen == "" {
		in.Listen = "0.0.0.0"
	}
	if in.Tag == "" {
		in.Tag = "vless-in"
	}
	enabled := 1
	if in.ID != 0 && !in.Enabled {
		enabled = 0
	}
	if in.ID == 0 {
		res, err := d.SQL.Exec(`INSERT INTO inbounds (node_id, remark, tag, protocol, listen, port, flow, dest, server_name, private_key, public_key, short_id, enabled, security, method, password)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			in.NodeID, in.Remark, in.Tag, in.Protocol, in.Listen, in.Port, in.Flow, in.Dest, in.ServerName, in.PrivateKey, in.PublicKey, in.ShortID, enabled, in.Security, in.Method, in.Password)
		if err != nil {
			return err
		}
		in.ID, _ = res.LastInsertId()
		in.Enabled = true
		return nil
	}
	_, err := d.SQL.Exec(`UPDATE inbounds SET node_id=?, remark=?, tag=?, protocol=?, listen=?, port=?, flow=?, dest=?, server_name=?, private_key=?, public_key=?, short_id=?, enabled=?, security=?, method=?, password=? WHERE id=?`,
		in.NodeID, in.Remark, in.Tag, in.Protocol, in.Listen, in.Port, in.Flow, in.Dest, in.ServerName, in.PrivateKey, in.PublicKey, in.ShortID, enabled, in.Security, in.Method, in.Password, in.ID)
	return err
}

func (d *DB) DeleteInbound(id int64) error {
	n, err := d.SQL.Exec(`DELETE FROM inbounds WHERE id=?`, id)
	if err != nil {
		return err
	}
	aff, _ := n.RowsAffected()
	if aff == 0 {
		return sql.ErrNoRows
	}
	return nil
}

const outboundSelect = `SELECT o.id, o.node_id, o.remark, o.tag, o.protocol, o.address, o.port, o.uuid, o.flow, o.public_key, o.short_id, o.server_name, o.username, o.password, o.enabled, o.is_default, COALESCE(n.name, '') FROM outbounds o LEFT JOIN nodes n ON n.id = o.node_id`

func scanOutbound(s scanner) (models.Outbound, error) {
	var o models.Outbound
	var enabled, def int
	err := s.Scan(&o.ID, &o.NodeID, &o.Remark, &o.Tag, &o.Protocol, &o.Address, &o.Port, &o.UUID, &o.Flow, &o.PublicKey, &o.ShortID, &o.ServerName, &o.Username, &o.Password, &enabled, &def, &o.NodeName)
	if err != nil {
		return o, err
	}
	o.Enabled = enabled != 0
	o.IsDefault = def != 0
	return o, nil
}

func (d *DB) ListOutbounds() ([]models.Outbound, error) {
	rows, err := d.SQL.Query(outboundSelect + ` ORDER BY o.is_default DESC, o.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Outbound
	for rows.Next() {
		o, err := scanOutbound(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	if out == nil {
		out = []models.Outbound{}
	}
	return out, rows.Err()
}

func (d *DB) ListOutboundsForNode(nodeID int64) ([]models.Outbound, error) {
	all, err := d.ListOutbounds()
	if err != nil {
		return nil, err
	}
	var out []models.Outbound
	for _, o := range all {
		if o.NodeID == 0 || o.NodeID == nodeID {
			out = append(out, o)
		}
	}
	if out == nil {
		out = []models.Outbound{}
	}
	return out, nil
}

func (d *DB) GetOutbound(id int64) (*models.Outbound, error) {
	row := d.SQL.QueryRow(outboundSelect+` WHERE o.id = ?`, id)
	o, err := scanOutbound(row)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (d *DB) SaveOutbound(o *models.Outbound) error {
	if o.Tag == "" {
		o.Tag = o.Protocol
	}
	if o.Protocol == "" {
		o.Protocol = "freedom"
	}
	en, def := 0, 0
	if o.Enabled {
		en = 1
	}
	if o.IsDefault {
		def = 1
	}
	if o.ID == 0 {
		if !o.Enabled {
			en = 1
			o.Enabled = true
		}
		res, err := d.SQL.Exec(`INSERT INTO outbounds (node_id, remark, tag, protocol, address, port, uuid, flow, public_key, short_id, server_name, username, password, enabled, is_default)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			o.NodeID, o.Remark, o.Tag, o.Protocol, o.Address, o.Port, o.UUID, o.Flow, o.PublicKey, o.ShortID, o.ServerName, o.Username, o.Password, en, def)
		if err != nil {
			return err
		}
		o.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := d.SQL.Exec(`UPDATE outbounds SET node_id=?, remark=?, tag=?, protocol=?, address=?, port=?, uuid=?, flow=?, public_key=?, short_id=?, server_name=?, username=?, password=?, enabled=?, is_default=? WHERE id=?`,
		o.NodeID, o.Remark, o.Tag, o.Protocol, o.Address, o.Port, o.UUID, o.Flow, o.PublicKey, o.ShortID, o.ServerName, o.Username, o.Password, en, def, o.ID)
	return err
}

func (d *DB) DeleteOutbound(id int64) error {
	_, err := d.SQL.Exec(`DELETE FROM outbounds WHERE id=? AND is_default=0`, id)
	return err
}

func (d *DB) EnsureBuiltinOutbounds() error {
	var n int
	if err := d.SQL.QueryRow(`SELECT COUNT(*) FROM outbounds`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := d.SQL.Exec(`INSERT INTO outbounds (node_id, remark, tag, protocol, enabled, is_default) VALUES
	(0, '直连', 'direct', 'freedom', 1, 1),
	(0, '阻断', 'block', 'blackhole', 1, 0)`)
	return err
}

const nodeSelect = `SELECT id, name, token, arch, host, public_host, port, relay_node_id, relay_uuid, is_local, enabled, subscribe, xray_running, xray_message, config_rev, version, desired_version, force_update, last_seen, created_at FROM nodes`

func scanNode(s scanner) (models.Node, error) {
	var n models.Node
	var last sql.NullString
	var force, isLocal, enabled, subscribe, xrayRun int
	var relay sql.NullInt64
	err := s.Scan(&n.ID, &n.Name, &n.Token, &n.Arch, &n.Host, &n.PublicHost, &n.Port, &relay, &n.RelayUUID,
		&isLocal, &enabled, &subscribe, &xrayRun, &n.XrayMessage, &n.ConfigRev, &n.Version, &n.DesiredVer, &force, &last, &n.CreatedAt)
	if err != nil {
		return n, err
	}
	n.ForceUpdate = force != 0
	n.IsLocal = isLocal != 0
	n.Enabled = enabled != 0
	n.Subscribe = subscribe != 0
	n.XrayRunning = xrayRun != 0
	if relay.Valid && relay.Int64 > 0 {
		id := relay.Int64
		n.RelayNodeID = &id
	}
	if n.Port == 0 {
		n.Port = 443
	}
	if last.Valid && last.String != "" {
		if t, err := parseTime(last.String); err == nil {
			n.LastSeen = &t
			n.Online = time.Since(t) < 90*time.Second
		}
	}
	if n.IsLocal {
		n.Online = true
	}
	return n, nil
}

func (d *DB) ListNodes() ([]models.Node, error) {
	rows, err := d.SQL.Query(nodeSelect + ` ORDER BY is_local DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	if out == nil {
		out = []models.Node{}
	}
	return out, rows.Err()
}

func (d *DB) GetNode(id int64) (*models.Node, error) {
	row := d.SQL.QueryRow(nodeSelect+` WHERE id = ?`, id)
	n, err := scanNode(row)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (d *DB) GetNodeByToken(token string) (*models.Node, error) {
	row := d.SQL.QueryRow(nodeSelect+` WHERE token = ?`, token)
	n, err := scanNode(row)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (d *DB) LocalNode() (*models.Node, error) {
	row := d.SQL.QueryRow(nodeSelect + ` WHERE is_local=1 ORDER BY id LIMIT 1`)
	n, err := scanNode(row)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (d *DB) CreateNode(n models.Node) (int64, error) {
	if n.Port == 0 {
		n.Port = 443
	}
	local, enabled, sub := 0, 1, 1
	if n.IsLocal {
		local = 1
	}
	if !n.Enabled {
		enabled = 0
	}
	if !n.Subscribe {
		sub = 0
	}
	var relay any
	if n.RelayNodeID != nil && *n.RelayNodeID > 0 {
		relay = *n.RelayNodeID
	}
	res, err := d.SQL.Exec(`INSERT INTO nodes (name, token, public_host, port, relay_node_id, relay_uuid, is_local, enabled, subscribe)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.Name, n.Token, n.PublicHost, n.Port, relay, n.RelayUUID, local, enabled, sub)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) UpdateNode(n models.Node) error {
	enabled, sub, local := 0, 0, 0
	if n.Enabled {
		enabled = 1
	}
	if n.Subscribe {
		sub = 1
	}
	if n.IsLocal {
		local = 1
	}
	var relay any
	if n.RelayNodeID != nil && *n.RelayNodeID > 0 {
		relay = *n.RelayNodeID
	}
	_, err := d.SQL.Exec(`UPDATE nodes SET name=?, public_host=?, port=?, relay_node_id=?, relay_uuid=?, is_local=?, enabled=?, subscribe=? WHERE id=?`,
		n.Name, n.PublicHost, n.Port, relay, n.RelayUUID, local, enabled, sub, n.ID)
	return err
}

func (d *DB) DeleteNode(id int64) error {
	n, err := d.GetNode(id)
	if err != nil {
		return err
	}
	if n.IsLocal {
		return fmt.Errorf("本机节点不能删除")
	}
	if _, err = d.SQL.Exec(`DELETE FROM inbounds WHERE node_id = ?`, id); err != nil {
		return err
	}
	_, err = d.SQL.Exec(`DELETE FROM nodes WHERE id = ?`, id)
	return err
}

func (d *DB) TouchNode(id int64, version, arch, host string, xrayRunning bool, xrayMsg string) error {
	run := 0
	if xrayRunning {
		run = 1
	}
	_, err := d.SQL.Exec(`UPDATE nodes SET version=?, arch=?, host=?, last_seen=?, xray_running=?, xray_message=? WHERE id=?`,
		version, arch, host, time.Now().UTC().Format(time.RFC3339), run, xrayMsg, id)
	return err
}

func (d *DB) SetNodeConfigRev(id int64, rev string) error {
	_, err := d.SQL.Exec(`UPDATE nodes SET config_rev=? WHERE id=?`, rev, id)
	return err
}

func (d *DB) BumpConfigRev() (string, error) {
	rev := time.Now().UTC().Format("20060102150405")
	_, err := d.SQL.Exec(`UPDATE nodes SET config_rev=?`, rev)
	return rev, err
}

func (d *DB) SetNodeForce(id int64, desired string, force bool) error {
	v := 0
	if force {
		v = 1
	}
	_, err := d.SQL.Exec(`UPDATE nodes SET desired_version=?, force_update=? WHERE id=?`, desired, v, id)
	return err
}

func (d *DB) ClearNodeForce(id int64) error {
	_, err := d.SQL.Exec(`UPDATE nodes SET force_update=0 WHERE id=?`, id)
	return err
}

func (d *DB) SetAllNodesForce(desired string) error {
	_, err := d.SQL.Exec(`UPDATE nodes SET desired_version=?, force_update=1`, desired)
	return err
}

func (d *DB) Stats() (userTotal, userEnabled, planTotal int, traffic int64, err error) {
	err = d.SQL.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&userTotal)
	if err != nil {
		return
	}
	err = d.SQL.QueryRow(`SELECT COUNT(*) FROM users WHERE enabled=1`).Scan(&userEnabled)
	if err != nil {
		return
	}
	err = d.SQL.QueryRow(`SELECT COUNT(*) FROM plans`).Scan(&planTotal)
	if err != nil {
		return
	}
	err = d.SQL.QueryRow(`SELECT COALESCE(SUM(traffic_up+traffic_down),0) FROM users`).Scan(&traffic)
	return
}

func IsUniqueErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique")
}
