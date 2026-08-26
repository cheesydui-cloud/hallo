package agentproto

type Heartbeat struct {
	NodeID  string `json:"node_id"`
	Token   string `json:"token"`
	Version string `json:"version"`
	Arch    string `json:"arch"`
	OS      string `json:"os"`
	Host    string `json:"host"`
}

type HeartbeatReply struct {
	OK           bool   `json:"ok"`
	DesiredVer   string `json:"desired_version"`
	UpdateURL    string `json:"update_url"`
	ForceUpdate  bool   `json:"force_update"`
	PanelVersion string `json:"panel_version"`
}

type PushResult struct {
	NodeID  string `json:"node_id"`
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}
