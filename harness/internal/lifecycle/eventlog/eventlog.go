package eventlog

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/layout"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

type Store struct {
	paths layout.Paths
}

type eventIndex struct {
	IDs     map[string]indexRecord
	Through int64
}

type indexRecord struct {
	ID         string `json:"id"`
	Offset     int64  `json:"offset"`
	NextOffset int64  `json:"next_offset"`
}

type DuplicateEventIDError struct {
	ID string
}

func (e *DuplicateEventIDError) Error() string {
	return fmt.Sprintf("event id %q already exists", e.ID)
}

func IsDuplicateEventID(err error) bool {
	var duplicate *DuplicateEventIDError
	return errors.As(err, &duplicate)
}

type CorruptLogError struct {
	Path string
	Line int
	Err  error
}

func (e *CorruptLogError) Error() string {
	return fmt.Sprintf("corrupt event log %s line %d: %v", e.Path, e.Line, e.Err)
}

func (e *CorruptLogError) Unwrap() error {
	return e.Err
}

func New(root string) (*Store, error) {
	paths, err := layout.Resolve(root)
	if err != nil {
		return nil, err
	}
	return &Store{paths: paths}, nil
}

func (s *Store) AppendJSON(data []byte) (schema.Event, error) {
	event, err := schema.DecodeEvent(data)
	if err != nil {
		return schema.Event{}, err
	}
	return event, s.Append(event)
}

func (s *Store) Append(event schema.Event) error {
	if err := schema.ValidateEvent(event); err != nil {
		return err
	}
	if _, err := layout.EnsureProject(s.paths.Root); err != nil {
		return err
	}

	return withLock(s.paths.EventLog+".lock", 5*time.Second, func() error {
		index, err := s.loadOrRebuildIndex()
		if err != nil {
			return err
		}
		if _, ok := index.IDs[event.ID]; ok {
			return &DuplicateEventIDError{ID: event.ID}
		}

		data, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("marshal event: %w", err)
		}
		line := append(data, '\n')
		file, err := os.OpenFile(s.paths.EventLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return fmt.Errorf("open event log: %w", err)
		}
		defer file.Close()
		offset, err := file.Seek(0, io.SeekEnd)
		if err != nil {
			return fmt.Errorf("seek event log: %w", err)
		}
		if index.Through != offset {
			if index, err = s.rebuildIndex(); err != nil {
				return err
			}
			if _, ok := index.IDs[event.ID]; ok {
				return &DuplicateEventIDError{ID: event.ID}
			}
			offset, err = file.Seek(0, io.SeekEnd)
			if err != nil {
				return fmt.Errorf("seek event log: %w", err)
			}
		}
		if _, err := file.Write(line); err != nil {
			return fmt.Errorf("append event: %w", err)
		}
		return s.appendIndexRecord(indexRecord{
			ID:         event.ID,
			Offset:     offset,
			NextOffset: offset + int64(len(line)),
		})
	})
}

// ReadAll returns every event in the log, oldest first. It is the canonical
// reader and runs WITHOUT the append lock, so it must stay consistent under
// concurrent writeback by other hosts: a final chunk with no terminating newline
// at EOF is an append in progress (a writer appends the whole "<json>\n" under
// the lock), so ReadAll treats the durable, newline-terminated prefix as the
// ledger and skips that partial — it will be complete on the next read. A
// newline-*terminated* malformed line is real corruption and still fails. This
// generalizes the surface's defensive read to any reader of the log.
func (s *Store) ReadAll() ([]schema.Event, error) {
	file, err := os.Open(s.paths.EventLog)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open event log: %w", err)
	}
	defer file.Close()

	reader := bufio.NewReaderSize(file, 64*1024)
	var events []schema.Event
	lineNo := 0
	for {
		line, readErr := reader.ReadBytes('\n')
		terminated := len(line) > 0 && line[len(line)-1] == '\n'
		if trimmed := bytes.TrimSpace(line); len(trimmed) > 0 {
			lineNo++
			if !terminated && errors.Is(readErr, io.EOF) {
				// In-progress trailing append by a concurrent writer: skip it.
				break
			}
			event, decodeErr := schema.DecodeEvent(trimmed)
			if decodeErr != nil {
				return events, &CorruptLogError{Path: s.paths.EventLog, Line: lineNo, Err: decodeErr}
			}
			events = append(events, event)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return events, fmt.Errorf("read event log: %w", readErr)
		}
	}
	return events, nil
}

func (s *Store) indexPath() string {
	return filepath.Join(s.paths.MnemonDir, "events.index")
}

func (s *Store) loadOrRebuildIndex() (eventIndex, error) {
	index, ok, err := s.loadIndex()
	if err != nil {
		return eventIndex{}, err
	}
	if ok {
		return index, nil
	}
	return s.rebuildIndex()
}

func (s *Store) loadIndex() (eventIndex, bool, error) {
	index := eventIndex{IDs: map[string]indexRecord{}}
	logSize, err := fileSize(s.paths.EventLog)
	if err != nil {
		return eventIndex{}, false, err
	}
	file, err := os.Open(s.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return index, logSize == 0, nil
		}
		return eventIndex{}, false, fmt.Errorf("open event index: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var record indexRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return index, false, nil
		}
		if record.ID == "" || record.Offset < 0 || record.NextOffset <= record.Offset {
			return index, false, nil
		}
		if _, exists := index.IDs[record.ID]; exists {
			return index, false, nil
		}
		index.IDs[record.ID] = record
		if record.NextOffset > index.Through {
			index.Through = record.NextOffset
		}
	}
	if err := scanner.Err(); err != nil {
		return eventIndex{}, false, fmt.Errorf("read event index: %w", err)
	}
	if index.Through != logSize {
		return index, false, nil
	}
	return index, true, nil
}

func (s *Store) rebuildIndex() (eventIndex, error) {
	index := eventIndex{IDs: map[string]indexRecord{}}
	file, err := os.Open(s.paths.EventLog)
	if err != nil {
		if os.IsNotExist(err) {
			if err := s.writeIndex(nil); err != nil {
				return eventIndex{}, err
			}
			return index, nil
		}
		return eventIndex{}, fmt.Errorf("open event log: %w", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var records []indexRecord
	var offset int64
	lineNo := 0
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			lineNo++
			nextOffset := offset + int64(len(line))
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) > 0 {
				event, decodeErr := schema.DecodeEvent(trimmed)
				if decodeErr != nil {
					return index, &CorruptLogError{Path: s.paths.EventLog, Line: lineNo, Err: decodeErr}
				}
				if _, exists := index.IDs[event.ID]; exists {
					return index, fmt.Errorf("event id %q already exists", event.ID)
				}
				record := indexRecord{ID: event.ID, Offset: offset, NextOffset: nextOffset}
				index.IDs[event.ID] = record
				records = append(records, record)
			}
			offset = nextOffset
			index.Through = offset
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			break
		}
		return index, fmt.Errorf("read event log: %w", err)
	}
	if err := s.writeIndex(records); err != nil {
		return eventIndex{}, err
	}
	return index, nil
}

func (s *Store) writeIndex(records []indexRecord) error {
	path := s.indexPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create event index parent: %w", err)
	}
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open event index temp: %w", err)
	}
	encodeErr := func() error {
		encoder := json.NewEncoder(file)
		for _, record := range records {
			if err := encoder.Encode(record); err != nil {
				return fmt.Errorf("encode event index: %w", err)
			}
		}
		return nil
	}()
	closeErr := file.Close()
	if encodeErr != nil {
		_ = os.Remove(tmp)
		return encodeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close event index temp: %w", closeErr)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace event index: %w", err)
	}
	return nil
}

func (s *Store) appendIndexRecord(record indexRecord) error {
	path := s.indexPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create event index parent: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open event index: %w", err)
	}
	defer file.Close()
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal event index record: %w", err)
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("append event index: %w", err)
	}
	return nil
}

func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("stat %s: %w", path, err)
	}
	return info.Size(), nil
}

func withLock(path string, timeout time.Duration, fn func() error) error {
	deadline := time.Now().Add(timeout)
	for {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = fmt.Fprintf(file, "%d\n", os.Getpid())
			_ = file.Close()
			defer os.Remove(path)
			return fn()
		}
		if !errors.Is(err, os.ErrExist) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("create lock parent: %w", err)
			}
			continue
		}
		// Recover a stale lock left by a crashed writer: if the recorded PID is
		// no longer alive, remove it and retry instead of wedging until timeout.
		if pid := readLockPID(path); pid > 0 && !processAlive(pid) {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for lock %s", path)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func readLockPID(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}

func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
