package reactor

import (
	"context"
	"errors"
	"fmt"
	"time"

	lifecyclestatus "github.com/mnemon-dev/mnemon/harness/internal/lifecycle/status"
)

const StatusRefreshID = "status.refresh"

var ErrNotFound = errors.New("reactor not found")

type Context struct {
	Root string
	Now  time.Time
}

type Reactor interface {
	Name() string
	Type() string
	Run(context.Context, Context) (Result, error)
}

type Registry struct {
	reactors map[string]Reactor
}

type Result struct {
	ReactorID string
	Outcome   string
	Message   string
	Status    lifecyclestatus.Result
}

func DefaultRegistry() Registry {
	return NewRegistry(StatusRefreshReactor{})
}

func NewRegistry(reactors ...Reactor) Registry {
	registry := Registry{reactors: map[string]Reactor{}}
	for _, item := range reactors {
		if item == nil || item.Name() == "" {
			continue
		}
		registry.reactors[item.Name()] = item
	}
	return registry
}

func (r Registry) Get(name string) (Reactor, bool) {
	item, ok := r.reactors[name]
	return item, ok
}

func (r Registry) Run(ctx context.Context, name string, run Context) (Result, error) {
	item, ok := r.Get(name)
	if !ok {
		return Result{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return item.Run(ctx, run)
}

type StatusRefreshReactor struct{}

func (StatusRefreshReactor) Name() string {
	return StatusRefreshID
}

func (StatusRefreshReactor) Type() string {
	return "deterministic"
}

func (StatusRefreshReactor) Run(_ context.Context, run Context) (Result, error) {
	return RunStatusRefresh(run.Root, run.Now)
}

func RunStatusRefresh(root string, now time.Time) (Result, error) {
	statusResult, err := lifecyclestatus.Refresh(root, now)
	if err != nil {
		return Result{}, err
	}
	return Result{
		ReactorID: StatusRefreshID,
		Outcome:   "completed",
		Message:   "status refreshed from lifecycle events",
		Status:    statusResult,
	}, nil
}

func DispatchStub(jobType string) Result {
	if jobType == "semantic" {
		return Result{
			Outcome: "blocked",
			Message: "semantic job requires HostAgent runner; runner dispatch is not implemented in this slice",
		}
	}
	return Result{
		Outcome: "skipped",
		Message: "no deterministic reactor matched the job",
	}
}
