package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// AssertionContext matches the inputs used by the Python assertion handlers.
type AssertionContext struct {
	Report       map[string]any
	WorkspaceDir string
	MnemonDir    string
	Env          map[string]string
}

type AssertionHandler interface {
	Assert(context.Context, AssertionContext) ([]AssertionResult, error)
}

type AssertionFunc func(context.Context, AssertionContext) ([]AssertionResult, error)

func (fn AssertionFunc) Assert(ctx context.Context, input AssertionContext) ([]AssertionResult, error) {
	if fn == nil {
		return nil, errors.New("assertion func is nil")
	}
	return fn(ctx, input)
}

// AssertionResult is the wire-compatible shape emitted by scripts/codex_app_server_eval.py.
type AssertionResult struct {
	Name     string         `json:"name"`
	Passed   bool           `json:"passed"`
	Expected any            `json:"expected,omitempty"`
	Rejected any            `json:"rejected,omitempty"`
	Path     string         `json:"path,omitempty"`
	Extra    map[string]any `json:"-"`
}

func (result AssertionResult) Validate() error {
	if strings.TrimSpace(result.Name) == "" {
		return errors.New("name is required")
	}
	return nil
}

func ValidateAssertionResults(results []AssertionResult) error {
	var errs []error
	for index, result := range results {
		if err := result.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("assertions[%d]: %w", index, err))
		}
	}
	return errors.Join(errs...)
}

func FailedAssertions(results []AssertionResult) []AssertionResult {
	var failed []AssertionResult
	for _, result := range results {
		if !result.Passed {
			failed = append(failed, result)
		}
	}
	return failed
}

func (result AssertionResult) MarshalJSON() ([]byte, error) {
	data := map[string]any{}
	for key, value := range result.Extra {
		if !knownAssertionResultKey(key) {
			data[key] = value
		}
	}
	data["name"] = result.Name
	data["passed"] = result.Passed
	if result.Expected != nil {
		data["expected"] = result.Expected
	}
	if result.Rejected != nil {
		data["rejected"] = result.Rejected
	}
	if result.Path != "" {
		data["path"] = result.Path
	}
	return json.Marshal(data)
}

func (result *AssertionResult) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("assertion result must be an object: %w", err)
	}

	name, err := requiredJSONString(raw, "name")
	if err != nil {
		return err
	}
	passed, err := requiredJSONBool(raw, "passed")
	if err != nil {
		return err
	}
	path, err := optionalJSONString(raw, "path")
	if err != nil {
		return err
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("decode assertion result: %w", err)
	}
	for key := range decoded {
		if knownAssertionResultKey(key) {
			delete(decoded, key)
		}
	}

	*result = AssertionResult{
		Name:   name,
		Passed: passed,
		Path:   path,
		Extra:  decoded,
	}
	if value, ok, err := optionalJSONAny(raw, "expected"); err != nil {
		return err
	} else if ok {
		result.Expected = value
	}
	if value, ok, err := optionalJSONAny(raw, "rejected"); err != nil {
		return err
	} else if ok {
		result.Rejected = value
	}
	return result.Validate()
}

func requiredJSONString(raw map[string]json.RawMessage, key string) (string, error) {
	value, ok := raw[key]
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}
	var decoded string
	if err := json.Unmarshal(value, &decoded); err != nil {
		return "", fmt.Errorf("%s must be a string", key)
	}
	if strings.TrimSpace(decoded) == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return decoded, nil
}

func optionalJSONString(raw map[string]json.RawMessage, key string) (string, error) {
	value, ok := raw[key]
	if !ok {
		return "", nil
	}
	var decoded string
	if err := json.Unmarshal(value, &decoded); err != nil {
		return "", fmt.Errorf("%s must be a string", key)
	}
	return decoded, nil
}

func requiredJSONBool(raw map[string]json.RawMessage, key string) (bool, error) {
	value, ok := raw[key]
	if !ok {
		return false, fmt.Errorf("%s is required", key)
	}
	var decoded bool
	if err := json.Unmarshal(value, &decoded); err != nil {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return decoded, nil
}

func optionalJSONAny(raw map[string]json.RawMessage, key string) (any, bool, error) {
	value, ok := raw[key]
	if !ok {
		return nil, false, nil
	}
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return nil, false, fmt.Errorf("%s must be valid JSON", key)
	}
	return decoded, true, nil
}

func knownAssertionResultKey(key string) bool {
	switch key {
	case "name", "passed", "expected", "rejected", "path":
		return true
	default:
		return false
	}
}
