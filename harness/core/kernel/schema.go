package kernel

import (
	"fmt"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
)

type SchemaGuard struct {
	Required map[contract.ResourceKind][]string
}

func DefaultSchemaGuard() SchemaGuard {
	return SchemaGuard{Required: map[contract.ResourceKind][]string{"memory": {"content"}, "goal": {"statement"}, "skill": {"name"}}}
}
func (g SchemaGuard) Validate(kind contract.ResourceKind, fields map[string]any) error {
	required, known := g.Required[kind]
	if !known {
		return fmt.Errorf("%w: unknown resource kind %q", errSchema, kind)
	}
	for _, f := range required {
		if _, ok := fields[f]; !ok {
			return fmt.Errorf("%w: %s requires %q", errSchema, kind, f)
		}
	}
	return nil
}
