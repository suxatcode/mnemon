package remoteapi

const (
	DefaultAuthFileName = "auth.json"
	DefaultStoreName    = "default"
	RPCServiceName      = "Mnemon"
)

type Auth struct {
	Principal string
	Token     string
}

type Response struct {
	JSON []byte
	Text string
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

type CommonRequest struct {
	Auth Auth
}

type StatusRequest struct {
	Auth Auth
}

type RememberRequest struct {
	Auth       Auth
	Content    string
	Category   string
	Importance int
	Tags       string
	Source     string
	Entities   string
	EntityMode string
	NoDiff     bool
	Agent      string
}

type RecallRequest struct {
	Auth     Auth
	Query    string
	Category string
	Limit    int
	Source   string
	Basic    bool
	Intent   string
	Verbose  bool
}

type SearchRequest struct {
	Auth  Auth
	Query string
	Limit int
}

type LinkRequest struct {
	Auth     Auth
	SourceID string
	TargetID string
	Type     string
	Weight   float64
	MetaJSON string
}

type ForgetRequest struct {
	Auth Auth
	ID   string
}

type LogRequest struct {
	Auth  Auth
	Limit int
}

type RelatedRequest struct {
	Auth     Auth
	ID       string
	EdgeType string
	Depth    int
}

type GCRequest struct {
	Auth      Auth
	Threshold float64
	Limit     int
	KeepID    string
}

type ReceiptRequest struct {
	Auth  Auth
	Limit int
}

type EmbedRequest struct {
	Auth   Auth
	ID     string
	All    bool
	Status bool
}

type ImportRequest struct {
	Auth   Auth
	Draft  []byte
	NoDiff bool
	DryRun bool
	Agent  string
}

type VizRequest struct {
	Auth   Auth
	Format string
}
