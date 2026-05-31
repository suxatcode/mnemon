package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/declaration"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

var jobIDUnsafe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func (d *Daemon) enqueueDeclaredControllerJobs(events []schema.Event, now time.Time) (int, error) {
	if _, err := os.Stat(filepath.Join(d.paths.Root, "harness", "loops")); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("stat loop declarations: %w", err)
	}
	enqueued := 0
	for _, event := range events {
		loopName := eventString(event.Loop)
		hostName := eventString(event.Host)
		if loopName == "" || hostName == "" {
			continue
		}
		loop, err := declaration.LoadLoop(d.paths.Root, loopName)
		if err != nil {
			return enqueued, err
		}
		binding, err := declaration.LoadBinding(d.paths.Root, hostName, loopName)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return enqueued, err
		}
		for _, controller := range loop.Controllers {
			if !controllerWatches(controller, event.Type) {
				continue
			}
			spec, ok := loop.Jobs[controller.Enqueue]
			if !ok {
				return enqueued, fmt.Errorf("controller %s references missing job %s", controller.Name, controller.Enqueue)
			}
			job, err := d.jobFromController(event, loop, binding, controller, spec, now)
			if err != nil {
				return enqueued, err
			}
			exists, err := d.jobExistsAnyStatus(job.ID)
			if err != nil {
				return enqueued, err
			}
			if exists {
				continue
			}
			if err := d.Enqueue(job); err != nil {
				return enqueued, err
			}
			enqueued++
		}
	}
	return enqueued, nil
}

func (d *Daemon) jobFromController(event schema.Event, loop declaration.LoopManifest, binding declaration.BindingManifest, controller declaration.LoopController, spec declaration.JobSpec, now time.Time) (Job, error) {
	runnerBinding := binding.RunnerBindings[controller.Enqueue]
	prompt, err := controllerPrompt(d.paths.Root, loop, spec, runnerBinding)
	if err != nil {
		return Job{}, err
	}
	jobType := spec.Type
	if jobType == "" {
		jobType = "semantic"
	}
	target := map[string]any{
		"loop":            loop.Name,
		"host":            binding.Host,
		"controller":      controller.Name,
		"source_event_id": event.ID,
		"reason":          controller.Reason,
		"prompt":          prompt,
	}
	addRunnerTarget(target, runnerBinding)
	budget := map[string]any{}
	if spec.MaxTurns > 0 {
		budget["max_turns"] = spec.MaxTurns
	}
	return Job{
		SchemaVersion: JobSchemaVersion,
		ID:            controllerJobID(controller.Name, event.ID),
		Type:          jobType,
		ReactorID:     controller.Enqueue,
		JobSpecRef:    controller.Enqueue,
		Target:        target,
		Priority:      "normal",
		Status:        "queued",
		DueAt:         now.UTC().Format(time.RFC3339),
		MaxAttempts:   3,
		Budget:        budget,
		EvidenceRefs:  []string{event.ID},
		CorrelationID: event.CorrelationID,
		UpdatedAt:     now.UTC().Format(time.RFC3339),
	}, nil
}

func controllerPrompt(root string, loop declaration.LoopManifest, spec declaration.JobSpec, runnerBinding declaration.RunnerBinding) (string, error) {
	prompt := spec.Prompt
	promptFrom := runnerBinding.PromptFrom
	if promptFrom == "" {
		promptFrom = spec.Spec
	}
	if promptFrom == "" {
		return prompt, nil
	}
	data, err := os.ReadFile(filepath.Join(root, "harness", "loops", loop.Name, filepath.FromSlash(promptFrom)))
	if err != nil {
		return "", fmt.Errorf("read job prompt %s: %w", promptFrom, err)
	}
	if prompt == "" {
		return string(data), nil
	}
	return prompt + "\n\n" + string(data), nil
}

func addRunnerTarget(target map[string]any, runnerBinding declaration.RunnerBinding) {
	if runnerBinding.Mode != "" {
		target["runner_mode"] = runnerBinding.Mode
	}
	if runnerBinding.Runner != "" {
		target["runner_id"] = runnerBinding.Runner
	}
	if runnerBinding.Agent != "" {
		target["agent"] = runnerBinding.Agent
	}
	if runnerBinding.PromptFrom != "" {
		target["prompt_from"] = runnerBinding.PromptFrom
	}
	if runnerBinding.FallbackRunner != "" {
		target["fallback_runner"] = runnerBinding.FallbackRunner
	}
}

func controllerWatches(controller declaration.LoopController, eventType string) bool {
	for _, watch := range controller.Watches {
		if watch == eventType {
			return true
		}
	}
	return false
}

func (d *Daemon) jobExistsAnyStatus(jobID string) (bool, error) {
	for _, statusValue := range []string{"queued", "completed", "failed", "blocked", "skipped"} {
		if _, err := os.Stat(d.jobPath(statusValue, jobID)); err == nil {
			return true, nil
		} else if !os.IsNotExist(err) {
			return false, fmt.Errorf("stat job %s/%s: %w", statusValue, jobID, err)
		}
	}
	return false, nil
}

func controllerJobID(controllerName, eventID string) string {
	id := "job_" + sanitizeJobID(controllerName) + "_" + sanitizeJobID(eventID)
	return strings.Trim(id, "_")
}

func sanitizeJobID(value string) string {
	value = jobIDUnsafe.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	if value == "" {
		return "unknown"
	}
	return value
}

func eventString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
