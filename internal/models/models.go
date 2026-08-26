package models

import "time"

type Admin struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type Plan struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	TrafficLimit int64     `json:"traffic_limit"` // bytes; 0 = unlimited
	DurationDays int       `json:"duration_days"` // 0 = unlimited
	Note         string    `json:"note"`
	CreatedAt    time.Time `json:"created_at"`
}

type User struct {
	ID          int64      `json:"id"`
	Email       string     `json:"email"`
	Remark      string     `json:"remark"`
	UUID        string     `json:"uuid"`
	PlanID      *int64     `json:"plan_id"`
	PlanName    string     `json:"plan_name,omitempty"`
	ExpireAt    *time.Time `json:"expire_at"`
	TrafficUp   int64      `json:"traffic_up"`
	TrafficDown int64      `json:"traffic_down"`
	Enabled     bool       `json:"enabled"`
	SubToken    string     `json:"sub_token"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (u User) TrafficTotal() int64 {
	return u.TrafficUp + u.TrafficDown
}

type Inbound struct {
	ID         int64  `json:"id"`
	Protocol   string `json:"protocol"`
	Listen     string `json:"listen"`
	Port       int    `json:"port"`
	Flow       string `json:"flow"`
	Dest       string `json:"dest"`
	ServerName string `json:"server_name"`
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
	ShortID    string `json:"short_id"`
}

type Settings struct {
	PublicURL string `json:"public_url"`
	Listen    string `json:"listen"`
	XrayPath  string `json:"xray_path"`
	PanelHost string `json:"panel_host"`
}

type Dashboard struct {
	SetupNeeded  bool   `json:"setup_needed"`
	UserTotal    int    `json:"user_total"`
	UserEnabled  int    `json:"user_enabled"`
	PlanTotal    int    `json:"plan_total"`
	TrafficTotal int64  `json:"traffic_total"`
	XrayRunning  bool   `json:"xray_running"`
	XrayPath     string `json:"xray_path"`
	XrayMessage  string `json:"xray_message"`
	PublicURL    string `json:"public_url"`
	InboundPort  int    `json:"inbound_port"`
}
