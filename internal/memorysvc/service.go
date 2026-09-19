package memorysvc

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/suxatcode/mnemon/internal/model"
	"github.com/suxatcode/mnemon/internal/store"
)

const (
	RoleUser = model.RoleUser
	RoleOrg  = model.RoleOrg
)

// Actor is the authenticated (or local) caller. On the server, Principal and Role
// come from a verified JWT sub/role — never from the request body.
type Actor struct {
	Principal string
	Role      string
	Agent     string
}

func (a Actor) layer() string {
	if a.Role == RoleOrg {
		return model.LayerOrg
	}
	return model.LayerPersonal
}

// Options configures a memory service.
type Options struct {
	EmbedModel  string
	MaxInsights int
	EnforceACL  bool
	StoreName   string
}

// Service is the single Memory implementation used by the local CLI and mnemon-server.
type Service struct {
	db          *store.DB
	embedModel  string
	maxInsights int
	enforceACL  bool
	storeName   string
}

// New wraps an already-open store. The caller owns Close.
func New(db *store.DB, opts Options) *Service {
	max := opts.MaxInsights
	if max <= 0 {
		max = store.MaxInsights
	}
	name := opts.StoreName
	if name == "" {
		name = store.DefaultStoreName
	}
	return &Service{
		db:          db,
		embedModel:  opts.EmbedModel,
		maxInsights: max,
		enforceACL:  opts.EnforceACL,
		storeName:   name,
	}
}

func (s *Service) DB() *store.DB { return s.db }

func (s *Service) assertWritable() error {
	if s.db.IsReadOnly() {
		return fmt.Errorf("database is read-only")
	}
	return nil
}

// Result is JSON (or text) plus warnings that belong on the client stderr.
type Result struct {
	JSON     []byte
	Text     string
	Warnings []string
}

func (s *Service) encode(v any, warnings []string) (Result, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return Result{}, err
	}
	return Result{JSON: append(b, '\n'), Warnings: warnings}, nil
}

func (s *Service) textResult(text string, warnings []string) Result {
	return Result{Text: text, Warnings: warnings}
}

func (s *Service) canMutate(actor Actor, ins *model.Insight) error {
	if !s.enforceACL {
		return nil
	}
	if ins == nil {
		return fmt.Errorf("insight not found")
	}
	if ins.Layer == model.LayerOrg {
		if actor.Role != RoleOrg {
			return fmt.Errorf("forbidden: organizational memories can only be changed by the organization principal")
		}
		return nil
	}
	if ins.OwnerPrincipal != actor.Principal {
		return fmt.Errorf("forbidden: you can only change your own memories")
	}
	return nil
}

func (s *Service) diffOwner(actor Actor) (owner, layer string) {
	if !s.enforceACL {
		return "", ""
	}
	return actor.Principal, actor.layer()
}

func (s *Service) pruneOwner(actor Actor) string {
	if !s.enforceACL {
		return ""
	}
	if actor.Role == RoleOrg {
		return "" // org layer is not auto-pruned
	}
	return actor.Principal
}

func stripForgedTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		lower := strings.ToLower(tag)
		if strings.HasPrefix(lower, "principal:") || strings.HasPrefix(lower, "agent:") {
			continue
		}
		out = append(out, tag)
	}
	return out
}

func addProvenanceTags(tags []string, principal, agent string) []string {
	tags = stripForgedTags(tags)
	seen := make(map[string]bool, len(tags)+2)
	out := make([]string, 0, len(tags)+2)
	for _, tag := range tags {
		if !seen[tag] {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	for _, tag := range []string{"principal:" + principal, "agent:" + agent} {
		if strings.HasSuffix(tag, ":") {
			continue
		}
		if !seen[tag] {
			out = append(out, tag)
		}
	}
	return out
}

func parseCSV(raw string, maxItems, maxLen int, label string) ([]string, error) {
	var out []string
	if raw != "" {
		for _, item := range strings.Split(raw, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if len(item) > maxLen {
				return nil, fmt.Errorf("%s too long (%d chars, max %d)", label, len(item), maxLen)
			}
			out = append(out, item)
		}
		if len(out) > maxItems {
			return nil, fmt.Errorf("too many %ss (%d, max %d)", label, len(out), maxItems)
		}
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
}

func truncID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func warnf(warnings *[]string, format string, args ...any) {
	*warnings = append(*warnings, fmt.Sprintf(format, args...))
}
