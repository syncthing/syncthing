package matchers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/onsi/gomega/format"
)

type MatchJSONMatcher struct {
	JSONToMatch      interface{}
	firstFailurePath []interface{}
}

func (matcher *MatchJSONMatcher) Match(actual interface{}) (success bool, err error) {
	actualString, expectedString, err := matcher.prettyPrint(actual)
	if err != nil {
		return false, err
	}

	var aval interface{}
	var eval interface{}

	// this is guarded by prettyPrint
	json.Unmarshal([]byte(actualString), &aval)
	json.Unmarshal([]byte(expectedString), &eval)
	var equal bool
	equal, matcher.firstFailurePath = deepEqual(aval, eval)
	return equal, nil
}

func (matcher *MatchJSONMatcher) FailureMessage(actual interface{}) (message string) {
	actualString, expectedString, _ := matcher.prettyPrint(actual)
	return formattedMessage(format.Message(actualString, "to match JSON of", expectedString), matcher.firstFailurePath)
}

func (matcher *MatchJSONMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	actualString, expectedString, _ := matcher.prettyPrint(actual)
	return formattedMessage(format.Message(actualString, "not to match JSON of", expectedString), matcher.firstFailurePath)
}

func formattedMessage(comparisonMessage string, failurePath []interface{}) string {
	var diffMessage string
	if len(failurePath) == 0 {
		diffMessage = ""
	} else {
		diffMessage = fmt.Sprintf("\n\nfirst mismatched key: %s", formattedFailurePath(failurePath))
	}
	return fmt.Sprintf("%s%s", comparisonMessage, diffMessage)
}

func formattedFailurePath(failurePath []interface{}) string {
	formattedPaths := []string{}
	for i := len(failurePath) - 1; i >= 0; i-- {
		switch p := failurePath[i].(type) {
		case int:
			formattedPaths = append(formattedPaths, fmt.Sprintf(`[%d]`, p))
		default:
			if i != len(failurePath)-1 {
				formattedPaths = append(formattedPaths, ".")
			}
			formattedPaths = append(formattedPaths, fmt.Sprintf(`"%s"`, p))
		}
	}
	return strings.Join(formattedPaths, "")
}

func (matcher *MatchJSONMatcher) prettyPrint(actual interface{}) (actualFormatted, expectedFormatted string, err error) {
	actualString, ok := toString(actual)
	if !ok {
		return "", "", fmt.Errorf("MatchJSONMatcher matcher requires a string, stringer, or []byte.  Got actual:\n%s", format.Object(actual, 1))
	}
	expectedString, ok := toString(matcher.JSONToMatch)
	if !ok {
		return "", "", fmt.Errorf("MatchJSONMatcher matcher requires a string, stringer, or []byte.  Got expected:\n%s", format.Object(matcher.JSONToMatch, 1))
	}

	abuf := new(bytes.Buffer)
	ebuf := new(bytes.Buffer)

	if err := json.Indent(abuf, []byte(actualString), "", "  "); err != nil {
		return "", "", fmt.Errorf("Actual '%s' should be valid JSON, but it is not.\nUnderlying error:%s", actualString, err)
	}

	if err := json.Indent(ebuf, []byte(expectedString), "", "  "); err != nil {
		return "", "", fmt.Errorf("Expected '%s' should be valid JSON, but it is not.\nUnderlying error:%s", expectedString, err)
	}

	return abuf.String(), ebuf.String(), nil
}

func deepEqual(a interface{}, b interface{}) (bool, []interface{}) {
	var errorPath []interface{}
	if reflect.TypeOf(a) != reflect.TypeOf(b) {
		return false, errorPath
	}

	switch a.(type) {
	case []interface{}:
		if len(a.([]interface{})) != len(b.([]interface{})) {
			return false, errorPath
		}

		for i, v := range a.([]interface{}) {
			elementEqual, keyPath := deepEqual(v, b.([]interface{})[i])
			if !elementEqual {
				return false, append(keyPath, i)
			}
		}
		return true, errorPath

	case map[string]interface{}:
		if len(a.(map[string]interface{})) != len(b.(map[string]interface{})) {
			return false, errorPath
		}

		for k, v1 := range a.(map[string]interface{}) {
			v2, ok := b.(map[string]interface{})[k]
			if !ok {
				return false, errorPath
			}
			elementEqual, keyPath := deepEqual(v1, v2)
			if !elementEqual {
				return false, append(keyPath, k)
			}
		}
		return true, errorPath

	default:
		return a == b, errorPath
	}
}
