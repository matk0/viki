package specsync

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"strings"

	messages "github.com/cucumber/messages/go/v30"
)

// ValidateMessages independently binds the retained execution stream to the canonical
// sources and every passing result (including hooks, real step matches and Rust fixture receipts).
func ValidateMessages(raw []byte, snapshot Snapshot) error {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	pickles := map[string]*messages.Pickle{}
	cases := map[string]*messages.TestCase{}
	runs := map[string]*messages.TestCaseStarted{}
	finished := map[string]*messages.TestCaseFinished{}
	definitions := map[string]bool{}
	sources := map[string]string{}
	events := map[string]map[string]*messages.TestStepResult{}
	attestations := map[string][]string{}
	starts, ends := 0, 0
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var e messages.Envelope
		if err := json.Unmarshal(line, &e); err != nil {
			return fmt.Errorf("invalid Cucumber message")
		}
		switch {
		case e.ParseError != nil || e.UndefinedParameterType != nil:
			return fmt.Errorf("invalid executable source")
		case e.Source != nil:
			if _, exists := sources[e.Source.Uri]; exists {
				return fmt.Errorf("duplicate executed source")
			}
			sources[e.Source.Uri] = e.Source.Data
		case e.TestRunStarted != nil:
			starts++
		case e.TestRunFinished != nil:
			ends++
			if !e.TestRunFinished.Success || e.TestRunFinished.Exception != nil {
				return fmt.Errorf("unsuccessful Cucumber run")
			}
		case e.Pickle != nil:
			if pickles[e.Pickle.Id] != nil {
				return fmt.Errorf("duplicate pickle")
			}
			pickles[e.Pickle.Id] = e.Pickle
		case e.TestCase != nil:
			if cases[e.TestCase.Id] != nil {
				return fmt.Errorf("duplicate case")
			}
			cases[e.TestCase.Id] = e.TestCase
		case e.TestCaseStarted != nil:
			if runs[e.TestCaseStarted.Id] != nil || e.TestCaseStarted.Attempt != 0 {
				return fmt.Errorf("duplicate or retried case")
			}
			runs[e.TestCaseStarted.Id] = e.TestCaseStarted
		case e.TestCaseFinished != nil:
			if finished[e.TestCaseFinished.TestCaseStartedId] != nil || e.TestCaseFinished.WillBeRetried {
				return fmt.Errorf("duplicate or retried case completion")
			}
			finished[e.TestCaseFinished.TestCaseStartedId] = e.TestCaseFinished
		case e.StepDefinition != nil:
			if definitions[e.StepDefinition.Id] {
				return fmt.Errorf("duplicate step definition")
			}
			definitions[e.StepDefinition.Id] = true
		case e.TestStepFinished != nil:
			st := e.TestStepFinished
			if st.TestStepResult == nil || st.TestStepResult.Status != "PASSED" || st.TestStepResult.Exception != nil {
				return fmt.Errorf("non-passing step or hook")
			}
			if events[st.TestCaseStartedId] == nil {
				events[st.TestCaseStartedId] = map[string]*messages.TestStepResult{}
			}
			if events[st.TestCaseStartedId][st.TestStepId] != nil {
				return fmt.Errorf("duplicate step result")
			}
			events[st.TestCaseStartedId][st.TestStepId] = st.TestStepResult
		case e.TestRunHookFinished != nil:
			result := e.TestRunHookFinished.Result
			if result == nil || result.Status != "PASSED" || result.Exception != nil {
				return fmt.Errorf("non-passing run hook")
			}
		case e.Attachment != nil:
			if e.Attachment.MediaType == "application/vnd.vozokany.fixture+json" {
				attestations[e.Attachment.TestCaseStartedId] = append(attestations[e.Attachment.TestCaseStartedId], e.Attachment.Body)
			}
		}
	}
	if scanner.Err() != nil {
		return fmt.Errorf("Cucumber message limit exceeded")
	}
	if starts != 1 || ends != 1 || len(pickles) != len(snapshot.Scenarios) || len(cases) != len(pickles) || len(runs) != len(cases) || len(finished) != len(runs) {
		return fmt.Errorf("incomplete or filtered Cucumber execution")
	}
	featurePaths := map[string]Document{}
	for _, d := range snapshot.Documents {
		if d.Kind == "feature" {
			featurePaths[d.Path] = d
			if source, ok := sources[d.Path]; !ok || source != d.Content {
				return fmt.Errorf("executed feature source differs")
			}
		}
	}
	if len(sources) != len(featurePaths) {
		return fmt.Errorf("executed feature sets differ")
	}
	expected := map[string]Scenario{}
	for _, s := range snapshot.Scenarios {
		expected[s.ID] = s
	}
	seen := map[string]bool{}
	results := []Result{}
	for id, run := range runs {
		tc := cases[run.TestCaseId]
		if tc == nil || finished[id] == nil {
			return fmt.Errorf("execution without completed case")
		}
		pickle := pickles[tc.PickleId]
		if pickle == nil {
			return fmt.Errorf("execution without source pickle")
		}
		ids := []string{}
		for _, tag := range pickle.Tags {
			if strings.HasPrefix(tag.Name, "@id:VOZ-SC-") {
				ids = append(ids, strings.TrimPrefix(tag.Name, "@id:"))
			}
		}
		if len(ids) != 1 || seen[ids[0]] {
			return fmt.Errorf("missing or duplicate executed scenario")
		}
		scenario, ok := expected[ids[0]]
		if !ok {
			return fmt.Errorf("unrecognized executed scenario")
		}
		seen[scenario.ID] = true
		feature, ok := featurePaths[pickle.Uri]
		if !ok || feature.ID != scenario.FeatureID || pickle.Name != scenario.Name || pickle.Language != "en" || len(pickle.Steps) != len(scenario.Steps) {
			return fmt.Errorf("executed scenario source differs")
		}
		pickleSteps := map[string]*messages.PickleStep{}
		for i, step := range pickle.Steps {
			if step == nil || step.Argument != nil || step.Text != scenario.Steps[i].Text || pickleSteps[step.Id] != nil {
				return fmt.Errorf("executed steps differ")
			}
			pickleSteps[step.Id] = step
		}
		if len(events[id]) != len(tc.TestSteps) {
			return fmt.Errorf("missing or extra step/hook results")
		}
		duration := big.NewInt(0)
		steps := []ResultStep{}
		seenSteps := map[string]bool{}
		for _, step := range tc.TestSteps {
			result := events[id][step.Id]
			if result == nil || seenSteps[step.Id] {
				return fmt.Errorf("missing or duplicate test step")
			}
			seenSteps[step.Id] = true
			if result.Duration == nil || result.Duration.Seconds < 0 || result.Duration.Nanos < 0 || result.Duration.Nanos >= 1000000000 {
				return fmt.Errorf("invalid executed step duration")
			}
			d := new(big.Int).Mul(big.NewInt(result.Duration.Seconds), big.NewInt(1000000000))
			d.Add(d, big.NewInt(result.Duration.Nanos))
			duration.Add(duration, d)
			if step.PickleStepId != "" {
				if len(step.StepDefinitionIds) != 1 || !definitions[step.StepDefinitionIds[0]] {
					return fmt.Errorf("undefined or ambiguous executable step")
				}
				ps := pickleSteps[step.PickleStepId]
				if ps == nil {
					return fmt.Errorf("missing executable pickle step")
				}
				steps = append(steps, ResultStep{Text: ps.Text, Status: "passed"})
			} else if step.HookId == "" {
				return fmt.Errorf("unknown executable test step")
			}
		}
		if len(steps) != len(scenario.Steps) {
			return fmt.Errorf("missing executable source step")
		}
		for i, step := range steps {
			if step.Text != scenario.Steps[i].Text {
				return fmt.Errorf("executed step order differs")
			}
		}
		receipts := attestations[id]
		if len(receipts) != 1 {
			return fmt.Errorf("missing Rust fixture receipt")
		}
		var receipt struct {
			Backend string `json:"backend"`
		}
		if json.Unmarshal([]byte(receipts[0]), &receipt) != nil || receipt.Backend != "rust" {
			return fmt.Errorf("wrong fixture backend")
		}
		results = append(results, Result{ScenarioID: scenario.ID, Status: "passed", Duration: duration.String(), Steps: steps})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].ScenarioID < results[j].ScenarioID })
	expectedResults := append([]Result{}, snapshot.Verification.Cases...)
	sort.Slice(expectedResults, func(i, j int) bool { return expectedResults[i].ScenarioID < expectedResults[j].ScenarioID })
	if !reflect.DeepEqual(results, expectedResults) {
		return fmt.Errorf("execution stream and snapshot results differ")
	}
	return nil
}
