package docopt

import (
	"fmt"
	"os"

	"github.com/docopt/docopt-go"
)

func Parse(doc string) (opts map[string]interface{}) {
	opts, err := docopt.Parse(doc, nil, true, "1.2.3", false, false)
	if ue, ok := err.(*docopt.UserError); ok {
		if ue.Error() != "" {
			fmt.Fprintf(os.Stderr, "\n%s\n", ue)
		}
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing docopt: %#v\n", err)
		os.Exit(1)
	}
	return
}
