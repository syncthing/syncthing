package prometheus

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/prometheus/common/model"
)

// Labels represents a collection of label name -> value mappings. This type is
// commonly used with the With(Labels) and GetMetricWith(Labels) methods of
// metric vector Collectors, e.g.:
//     myVec.With(Labels{"code": "404", "method": "GET"}).Add(42)
//
// The other use-case is the specification of constant label pairs in Opts or to
// create a Desc.
type Labels map[string]string

// reservedLabelPrefix is a prefix which is not legal in user-supplied
// label names.
const reservedLabelPrefix = "__"

var errInconsistentCardinality = errors.New("inconsistent label cardinality")

func validateValuesInLabels(labels Labels, expectedNumberOfValues int) error {
	if len(labels) != expectedNumberOfValues {
		return errInconsistentCardinality
	}

	for name, val := range labels {
		if !utf8.ValidString(val) {
			return fmt.Errorf("label %s: value %q is not valid UTF-8", name, val)
		}
	}

	return nil
}

func validateLabelValues(vals []string, expectedNumberOfValues int) error {
	if len(vals) != expectedNumberOfValues {
		return errInconsistentCardinality
	}

	for _, val := range vals {
		if !utf8.ValidString(val) {
			return fmt.Errorf("label value %q is not valid UTF-8", val)
		}
	}

	return nil
}

func checkLabelName(l string) bool {
	return model.LabelName(l).IsValid() && !strings.HasPrefix(l, reservedLabelPrefix)
}
