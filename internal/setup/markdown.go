package setup

import (
	"os"
	"strings"
)

const (
	markerStart = "<!-- mnemon:start -->"
	markerEnd   = "<!-- mnemon:end -->"
)

// InjectMemoryBlock appends the template to filePath if the mnemon marker is absent.
// Creates the file if it doesn't exist. Returns true if the file was modified.
func InjectMemoryBlock(filePath string, template []byte) (bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}

	if strings.Contains(string(content), markerStart) {
		return false, nil // already present
	}

	var buf []byte
	if len(content) > 0 {
		buf = append(buf, content...)
		if !strings.HasSuffix(string(content), "\n") {
			buf = append(buf, '\n')
		}
		buf = append(buf, '\n')
	}
	buf = append(buf, template...)

	if err := os.WriteFile(filePath, buf, 0644); err != nil {
		return false, err
	}
	return true, nil
}

// EjectMemoryBlock removes everything between <!-- mnemon:start --> and <!-- mnemon:end --> inclusive.
// Returns true if the file was modified.
func EjectMemoryBlock(filePath string) (bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	s := string(content)
	startIdx := strings.Index(s, markerStart)
	if startIdx < 0 {
		return false, nil
	}

	endIdx := strings.Index(s, markerEnd)
	if endIdx < 0 {
		return false, nil
	}
	endIdx += len(markerEnd)

	// Also remove a leading newline before the block if present
	if startIdx > 0 && s[startIdx-1] == '\n' {
		startIdx--
	}
	// Also remove a trailing newline after the block if present
	if endIdx < len(s) && s[endIdx] == '\n' {
		endIdx++
	}

	result := s[:startIdx] + s[endIdx:]
	result = strings.TrimSpace(result)

	// If nothing remains, remove the file entirely
	if result == "" {
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			return false, err
		}
		return true, nil
	}

	result += "\n"
	if err := os.WriteFile(filePath, []byte(result), 0644); err != nil {
		return false, err
	}
	return true, nil
}
