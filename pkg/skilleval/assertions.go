package skilleval

import (
	"regexp"
	"strings"
)

// AssertionResult holds the outcome of a single assertion check.
type AssertionResult struct {
	Type   string
	Value  string
	Passed bool
}

// RunAssertion checks a single assertion against output text and returns the result.
func RunAssertion(a Assertion, output string) AssertionResult {
	var passed bool
	switch a.Type {
	case "contains":
		passed = strings.Contains(output, a.Value)
	case "not_contains":
		passed = !strings.Contains(output, a.Value)
	case "matches":
		// Regex was validated at load time; MustCompile is safe here.
		passed = regexp.MustCompile(a.Value).MatchString(output)
	case "not_matches":
		passed = !regexp.MustCompile(a.Value).MatchString(output)
	}
	return AssertionResult{Type: a.Type, Value: a.Value, Passed: passed}
}

// RunAssertions runs all assertions against output. Returns results and overall pass/fail.
// All assertions must pass (AND semantics).
func RunAssertions(assertions []Assertion, output string) ([]AssertionResult, bool) {
	results := make([]AssertionResult, len(assertions))
	allPassed := true
	for i, a := range assertions {
		results[i] = RunAssertion(a, output)
		if !results[i].Passed {
			allPassed = false
		}
	}
	return results, allPassed
}
