package remoteapi

const (
	DefaultAuthFileName = "auth.json"
	DefaultStoreName    = "default"
)

type Envelope struct {
	Result   any      `json:"result,omitempty"`
	Text     string   `json:"text,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Error    string   `json:"error,omitempty"`
}

type Response struct {
	JSON     []byte
	Text     string
	Warnings []string
}

type Invite struct {
	SchemaVersion int    `json:"schema_version"`
	Name          string `json:"name,omitempty"`
	Server        string `json:"server"`
	Principal     string `json:"principal"`
	Token         string `json:"token"`
	CAPEM         string `json:"ca_pem,omitempty"`
	ServerName    string `json:"server_name,omitempty"`
	Workspace     string `json:"workspace,omitempty"`
	Role          string `json:"role,omitempty"`
}

type AuthConfig struct {
	SchemaVersion int            `json:"schema_version"`
	DefaultRemote string         `json:"default_remote,omitempty"`
	Remotes       []RemoteConfig `json:"remotes"`
}

type RemoteConfig struct {
	Name       string `json:"name"`
	Server     string `json:"server"`
	Principal  string `json:"principal"`
	TokenFile  string `json:"token_file"`
	CAFile     string `json:"ca_file,omitempty"`
	ServerName string `json:"server_name,omitempty"`
	Workspace  string `json:"workspace,omitempty"`
}

type RememberRequest struct {
	Content    string `json:"content"`
	Category   string `json:"category"`
	Importance int    `json:"importance"`
	Tags       string `json:"tags"`
	Source     string `json:"source"`
	Entities   string `json:"entities"`
	EntityMode string `json:"entity_mode"`
	NoDiff     bool   `json:"no_diff"`
}

type RecallRequest struct {
	Query    string `json:"query"`
	Category string `json:"category"`
	Limit    int    `json:"limit"`
	Source   string `json:"source"`
	Basic    bool   `json:"basic"`
	Intent   string `json:"intent"`
	Verbose  bool   `json:"verbose"`
}

type SearchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type LinkRequest struct {
	SourceID string  `json:"source_id"`
	TargetID string  `json:"target_id"`
	Type     string  `json:"type"`
	Weight   float64 `json:"weight"`
	MetaJSON string  `json:"meta_json"`
}

type ForgetRequest struct {
	ID string `json:"id"`
}

type LogRequest struct {
	Limit int `json:"limit"`
}

type RelatedRequest struct {
	ID       string `json:"id"`
	EdgeType string `json:"edge_type"`
	Depth    int    `json:"depth"`
}

type GCRequest struct {
	Threshold float64 `json:"threshold"`
	Limit     int     `json:"limit"`
	KeepID    string  `json:"keep_id"`
}

type ReceiptRequest struct {
	Limit int `json:"limit"`
}

type EmbedRequest struct {
	ID     string `json:"id"`
	All    bool   `json:"all"`
	Status bool   `json:"status"`
}

type ImportRequest struct {
	Draft  []byte `json:"draft"`
	NoDiff bool   `json:"no_diff"`
	DryRun bool   `json:"dry_run"`
}

type VizRequest struct {
	Format string `json:"format"`
}
