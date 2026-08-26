package agentproto

type Heartbeat struct {
	NodeID      string `json:"node_id"`
	Token       string `json:"token"`
	Version     string `json:"version"`
	Arch        string `json:"arch"`
	OS          string `json:"os"`
	Host        string `json:"host"`
	PublicIP    string `json:"public_ip"`
	XrayRunning bool   `json:"xray_running"`
	XrayMessage string `json:"xray_message"`
	ConfigRev   string `json:"config_rev"`
}

type HeartbeatReply struct {
	OK           bool           `json:"ok"`
	DesiredVer   string         `json:"desired_version"`
	UpdateURL    string         `json:"update_url"`
	ForceUpdate  bool           `json:"force_update"`
	PanelVersion string         `json:"panel_version"`
	ConfigRev    string         `json:"config_rev"`
	Config       map[string]any `json:"config,omitempty"`
	Node         NodeInfo       `json:"node"`
}

type NodeInfo struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	PublicHost string `json:"public_host"`
	Port       int    `json:"port"`
	Enabled    bool   `json:"enabled"`
}

type PushResult struct {
	NodeID  string `json:"node_id"`
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}
