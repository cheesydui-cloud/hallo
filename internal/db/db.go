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
  protocol TEXT NOT NULL DEFAULT 'vless',
  listen TEXT NOT NULL DEFAULT '0.0.0.0',
  port INTEGER NOT NULL DEFAULT 443,
  flow TEXT NOT NULL DEFAULT 'xtls-rprx-vision',
  dest TEXT NOT NULL,
  server_name TEXT NOT NULL,
  private_key TEXT NOT NULL,
  public_key TEXT NOT NULL,
  short_id TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`)
	return err
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
	return out, rows.Err()
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
		out = append(out, u)
	}
	if out == nil {
		out = []models.User{}
	}
	return out, nil
}

func (d *DB) GetInbound() (*models.Inbound, error) {
	row := d.SQL.QueryRow(`SELECT id, protocol, listen, port, flow, dest, server_name, private_key, public_key, short_id FROM inbounds ORDER BY id LIMIT 1`)
	in := &models.Inbound{}
	err := row.Scan(&in.ID, &in.Protocol, &in.Listen, &in.Port, &in.Flow, &in.Dest, &in.ServerName, &in.PrivateKey, &in.PublicKey, &in.ShortID)
	if err != nil {
		return nil, err
	}
	return in, nil
}

func (d *DB) SaveInbound(in models.Inbound) error {
	cur, err := d.GetInbound()
	if err == sql.ErrNoRows {
		_, err = d.SQL.Exec(`INSERT INTO inbounds (protocol, listen, port, flow, dest, server_name, private_key, public_key, short_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			in.Protocol, in.Listen, in.Port, in.Flow, in.Dest, in.ServerName, in.PrivateKey, in.PublicKey, in.ShortID)
		return err
	}
	if err != nil {
		return err
	}
	_, err = d.SQL.Exec(`UPDATE inbounds SET protocol=?, listen=?, port=?, flow=?, dest=?, server_name=?, private_key=?, public_key=?, short_id=? WHERE id=?`,
		in.Protocol, in.Listen, in.Port, in.Flow, in.Dest, in.ServerName, in.PrivateKey, in.PublicKey, in.ShortID, cur.ID)
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
