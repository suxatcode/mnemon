package metric

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Context struct {
	Root               string
	Now                time.Time
	BudgetUsedUSDToday float64
}

type Collector interface {
	Collect(context.Context, Context) (float64, error)
}

type CollectorFunc func(context.Context, Context) (float64, error)

func (fn CollectorFunc) Collect(ctx context.Context, input Context) (float64, error) {
	return fn(ctx, input)
}

type Registry map[string]Collector

func KnownNames() []string {
	return []string{
		"memory.lines",
		"memory.entries",
		"goal.idle_hours",
		"eventlog.size_mb",
		"audit.records",
		"proposal.open",
		"daemon.queue.depth",
		"daemon.budget.used_usd_today",
	}
}

func IsKnown(name string) bool {
	for _, known := range KnownNames() {
		if name == known {
			return true
		}
	}
	return false
}

func DefaultRegistry() Registry {
	return Registry{
		"memory.lines": CollectorFunc(func(ctx context.Context, input Context) (float64, error) {
			return lineCount(ctx, filepath.Join(cleanRoot(input.Root), "harness", "loops", "memory", "MEMORY.md"))
		}),
		"memory.entries": CollectorFunc(func(ctx context.Context, input Context) (float64, error) {
			return lineCount(ctx, filepath.Join(cleanRoot(input.Root), "harness", "loops", "memory", "MEMORY.md"))
		}),
		"goal.idle_hours": CollectorFunc(func(ctx context.Context, input Context) (float64, error) {
			latest, err := latestModTime(filepath.Join(cleanRoot(input.Root), ".mnemon", "harness", "goals"))
			if err != nil {
				return 0, err
			}
			if latest.IsZero() {
				return 0, nil
			}
			now := input.Now
			if now.IsZero() {
				now = time.Now().UTC()
			}
			return now.Sub(latest).Hours(), nil
		}),
		"eventlog.size_mb": CollectorFunc(func(ctx context.Context, input Context) (float64, error) {
			size, err := fileSize(filepath.Join(cleanRoot(input.Root), ".mnemon", "events.jsonl"))
			return float64(size) / 1024 / 1024, err
		}),
		"audit.records": CollectorFunc(func(ctx context.Context, input Context) (float64, error) {
			return fileCount(ctx, filepath.Join(cleanRoot(input.Root), ".mnemon", "harness", "audit", "records"))
		}),
		"proposal.open": CollectorFunc(func(ctx context.Context, input Context) (float64, error) {
			return fileCount(ctx, filepath.Join(cleanRoot(input.Root), ".mnemon", "harness", "proposals", "open"))
		}),
		"daemon.queue.depth": CollectorFunc(func(ctx context.Context, input Context) (float64, error) {
			return fileCount(ctx, filepath.Join(cleanRoot(input.Root), ".mnemon", "harness", "jobs", "queued"))
		}),
		"daemon.budget.used_usd_today": CollectorFunc(func(ctx context.Context, input Context) (float64, error) {
			return input.BudgetUsedUSDToday, nil
		}),
	}
}

func lineCount(ctx context.Context, path string) (float64, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var count float64
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}
		count++
	}
	return count, scanner.Err()
}

func fileCount(ctx context.Context, dir string) (float64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var count float64
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			count++
		}
	}
	return count, nil
}

func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	return info.Size(), nil
}

func latestModTime(dir string) (time.Time, error) {
	var latest time.Time
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
		return nil
	}); err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("walk %s: %w", dir, err)
	}
	return latest, nil
}

func cleanRoot(root string) string {
	if root == "" {
		return "."
	}
	return filepath.Clean(root)
}
