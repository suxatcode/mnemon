package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Suite struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Host        string   `json:"host,omitempty"`
	Lifecycle   string   `json:"lifecycle,omitempty"`
	Runner      string   `json:"runner,omitempty"`
	ScenarioIDs []string `json:"scenario_ids,omitempty"`
	Scenarios   []string `json:"scenarios,omitempty"`
	Rubrics     []string `json:"rubrics,omitempty"`
	Source      string   `json:"source,omitempty"`
}

type Scenario struct {
	ID               string   `json:"id"`
	Description      string   `json:"description,omitempty"`
	Area             string   `json:"area,omitempty"`
	Lifecycle        string   `json:"lifecycle,omitempty"`
	Loops            []string `json:"loops,omitempty"`
	ExpectedSkills   []string `json:"expected_skills,omitempty"`
	SetupHandler     string   `json:"setup_handler,omitempty"`
	AssertionHandler string   `json:"assertion_handler,omitempty"`
	AssertionBackend string   `json:"assertion_backend,omitempty"`
	Prompts          []string `json:"prompts,omitempty"`
	Source           string   `json:"source,omitempty"`
}

type RunPlan struct {
	Suite        Suite     `json:"suite"`
	ScenarioID   string    `json:"scenario_id"`
	Scenario     *Scenario `json:"scenario,omitempty"`
	ProjectLoops []string  `json:"project_loops"`
	Prompt       string    `json:"prompt"`
	Prompts      []string  `json:"prompts,omitempty"`
}

func BuildRunPlan(root, suiteName, scenarioID string) (RunPlan, error) {
	suite, err := LoadSuite(root, suiteName)
	if err != nil {
		return RunPlan{}, err
	}
	scenario, err := selectScenario(suite, scenarioID)
	if err != nil {
		return RunPlan{}, err
	}
	metadata, found, err := LoadScenario(root, scenario)
	if err != nil {
		return RunPlan{}, err
	}
	var scenarioMetadata *Scenario
	projectLoops := projectLoopsForScenario(scenario)
	prompt := promptForScenario(suite, scenario)
	prompts := []string{prompt}
	if found {
		scenarioMetadata = &metadata
		projectLoops = projectLoopsForMetadata(metadata)
		if len(metadata.Prompts) > 0 {
			prompts = append([]string(nil), metadata.Prompts...)
			prompt = metadata.Prompts[0]
		}
	}
	return RunPlan{
		Suite:        suite,
		ScenarioID:   scenario,
		Scenario:     scenarioMetadata,
		ProjectLoops: projectLoops,
		Prompt:       prompt,
		Prompts:      prompts,
	}, nil
}

func LoadSuite(root, name string) (Suite, error) {
	suites, err := ListSuites(root)
	if err != nil {
		return Suite{}, err
	}
	for _, suite := range suites {
		if suiteMatches(suite, name) {
			return suite, nil
		}
	}
	return Suite{}, fmt.Errorf("eval suite %q not found", name)
}

func ListSuites(root string) ([]Suite, error) {
	if root == "" {
		root = "."
	}
	root = filepath.Clean(root)
	matches, err := filepath.Glob(filepath.Join(root, "harness", "loops", "eval", "suites", "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob eval suites: %w", err)
	}
	var suites []Suite
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read eval suite %s: %w", path, err)
		}
		var suite Suite
		if err := json.Unmarshal(data, &suite); err != nil {
			return nil, fmt.Errorf("parse eval suite %s: %w", path, err)
		}
		if suite.Name == "" {
			return nil, fmt.Errorf("eval suite missing name: %s", path)
		}
		if len(suite.ScenarioIDs) == 0 && len(suite.Scenarios) == 0 {
			return nil, fmt.Errorf("eval suite missing scenario_ids or scenarios: %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		suite.Source = filepath.ToSlash(rel)
		suites = append(suites, suite)
	}
	sort.Slice(suites, func(i, j int) bool {
		return suites[i].Name < suites[j].Name
	})
	return suites, nil
}

func LoadScenario(root, id string) (Scenario, bool, error) {
	scenarios, err := ListScenarios(root)
	if err != nil {
		return Scenario{}, false, err
	}
	for _, scenario := range scenarios {
		if scenario.ID == id {
			return scenario, true, nil
		}
	}
	return Scenario{}, false, nil
}

func ListScenarios(root string) ([]Scenario, error) {
	if root == "" {
		root = "."
	}
	root = filepath.Clean(root)
	matches, err := filepath.Glob(filepath.Join(root, "harness", "loops", "eval", "scenarios", "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob eval scenarios: %w", err)
	}
	var scenarios []Scenario
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read eval scenario catalog %s: %w", path, err)
		}
		var catalog struct {
			Scenarios []Scenario `json:"scenarios"`
		}
		if err := json.Unmarshal(data, &catalog); err != nil {
			return nil, fmt.Errorf("parse eval scenario catalog %s: %w", path, err)
		}
		for _, scenario := range catalog.Scenarios {
			if scenario.ID == "" {
				return nil, fmt.Errorf("eval scenario catalog %s has scenario without id", path)
			}
			if len(scenario.Loops) == 0 {
				return nil, fmt.Errorf("eval scenario %q missing loops: %s", scenario.ID, path)
			}
			if len(scenario.Prompts) == 0 {
				return nil, fmt.Errorf("eval scenario %q missing prompts: %s", scenario.ID, path)
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				rel = path
			}
			scenario.Source = filepath.ToSlash(rel)
			scenarios = append(scenarios, scenario)
		}
	}
	sort.Slice(scenarios, func(i, j int) bool {
		return scenarios[i].ID < scenarios[j].ID
	})
	return scenarios, nil
}

func selectScenario(suite Suite, scenarioID string) (string, error) {
	scenarios := suiteScenarioIDs(suite)
	if len(scenarios) == 0 {
		return "", fmt.Errorf("eval suite %q has no scenarios", suite.Name)
	}
	if scenarioID == "" {
		return scenarios[0], nil
	}
	for _, scenario := range scenarios {
		if scenario == scenarioID {
			return scenario, nil
		}
	}
	return "", fmt.Errorf("scenario %q is not in eval suite %q", scenarioID, suite.Name)
}

func suiteScenarioIDs(suite Suite) []string {
	if len(suite.ScenarioIDs) > 0 {
		return suite.ScenarioIDs
	}
	return suite.Scenarios
}

func suiteMatches(suite Suite, name string) bool {
	name = strings.TrimSpace(name)
	if suite.Name == name {
		return true
	}
	sourceBase := strings.TrimSuffix(filepath.Base(suite.Source), filepath.Ext(suite.Source))
	return sourceBase == name
}

func projectLoopsForScenario(scenarioID string) []string {
	seen := map[string]bool{"eval": true}
	loops := []string{"eval"}
	for _, prefix := range []string{"memory", "skill", "goal"} {
		if strings.HasPrefix(scenarioID, prefix+"-") || strings.HasPrefix(scenarioID, prefix+"/") {
			if !seen[prefix] {
				loops = append(loops, prefix)
			}
		}
	}
	return loops
}

func projectLoopsForMetadata(scenario Scenario) []string {
	seen := map[string]bool{"eval": true}
	loops := []string{"eval"}
	for _, loop := range scenario.Loops {
		loop = strings.TrimSpace(loop)
		if loop == "" || seen[loop] {
			continue
		}
		seen[loop] = true
		loops = append(loops, loop)
	}
	return loops
}

func promptForScenario(suite Suite, scenarioID string) string {
	return fmt.Sprintf(
		"Run Mnemon eval suite %q scenario %q with host %q and runner %q. Treat this run as evidence only: collect artifacts, avoid mutating canonical eval assets, and summarize observed behavior against the declared suite rubrics.",
		suite.Name,
		scenarioID,
		suite.Host,
		suite.Runner,
	)
}
