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
	NodeIDs     []int64    `json:"node_ids"`
}

func (u User) TrafficTotal() int64 {
	return u.TrafficUp + u.TrafficDown
}

func (u User) Expired() bool {
	return u.ExpireAt != nil && time.Now().After(*u.ExpireAt)
}

type Inbound struct {
	ID         int64  `json:"id"`
	NodeID     int64  `json:"node_id"`
	Remark     string `json:"remark"`
	Tag        string `json:"tag"`
	Protocol   string `json:"protocol"` // vless / vmess / shadowsocks
	Listen     string `json:"listen"`
	Port       int    `json:"port"`
	Flow       string `json:"flow"`
	Security   string `json:"security"` // reality / none
	Dest       string `json:"dest"`
	ServerName string `json:"server_name"`
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
	ShortID    string `json:"short_id"`
	Method     string `json:"method"`
	Password   string `json:"password"`
	Enabled    bool   `json:"enabled"`
	NodeName   string `json:"node_name,omitempty"`
	ClientNum  int    `json:"client_num,omitempty"`
}

type Outbound struct {
	ID         int64  `json:"id"`
	NodeID     int64  `json:"node_id"` // 0 = 所有节点
	Remark     string `json:"remark"`
	Tag        string `json:"tag"`
	Protocol   string `json:"protocol"` // freedom / blackhole / vless / socks / http
	Address    string `json:"address"`
	Port       int    `json:"port"`
	UUID       string `json:"uuid"`
	Flow       string `json:"flow"`
	PublicKey  string `json:"public_key"`
	ShortID    string `json:"short_id"`
	ServerName string `json:"server_name"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Enabled    bool   `json:"enabled"`
	IsDefault  bool   `json:"is_default"`
	NodeName   string `json:"node_name,omitempty"`
}

type Settings struct {
	PublicURL string `json:"public_url"`
	Listen    string `json:"listen"`
	XrayPath  string `json:"xray_path"`
	PanelHost string `json:"panel_host"`
	Version   string `json:"version"`
	Repo      string `json:"repo"`
}

type Node struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Token       string     `json:"token"`
	Arch        string     `json:"arch"`
	Host        string     `json:"host"`
	PublicHost  string     `json:"public_host"`
	Port        int        `json:"port"`
	RelayNodeID *int64     `json:"relay_node_id"`
	RelayUUID   string     `json:"relay_uuid"`
	IsLocal     bool       `json:"is_local"`
	Enabled     bool       `json:"enabled"`
	Subscribe   bool       `json:"subscribe"`
	XrayRunning bool       `json:"xray_running"`
	XrayMessage string     `json:"xray_message"`
	ConfigRev   string     `json:"config_rev"`
	Version     string     `json:"version"`
	DesiredVer  string     `json:"desired_version"`
	ForceUpdate bool       `json:"force_update"`
	LastSeen    *time.Time `json:"last_seen"`
	Online      bool       `json:"online"`
	CreatedAt   time.Time  `json:"created_at"`
}

type Dashboard struct {
	SetupNeeded  bool   `json:"setup_needed"`
	UserTotal    int    `json:"user_total"`
	UserEnabled  int    `json:"user_enabled"`
	PlanTotal    int    `json:"plan_total"`
	NodeTotal    int    `json:"node_total"`
	NodeOnline   int    `json:"node_online"`
	TrafficTotal int64  `json:"traffic_total"`
	XrayRunning  bool   `json:"xray_running"`
	XrayPath     string `json:"xray_path"`
	XrayMessage  string `json:"xray_message"`
	PublicURL    string `json:"public_url"`
	InboundPort  int    `json:"inbound_port"`
}
