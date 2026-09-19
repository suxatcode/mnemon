package kernel

import (
	"errors"

	"github.com/suxatcode/mnemon/harness/internal/contract"
)

var (
	errSchema = errors.New("schema")
	errAuthz  = errors.New("authz")
)

type conflictError struct{ conflicts []contract.Conflict }

func (e *conflictError) Error() string { return "conflict" }
