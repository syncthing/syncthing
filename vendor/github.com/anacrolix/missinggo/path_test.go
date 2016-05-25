package missinggo

import (
	"fmt"
)

func ExamplePathSplitExt() {
	fmt.Printf("%q\n", PathSplitExt(".cshrc"))
	fmt.Printf("%q\n", PathSplitExt("dir/a.ext"))
	fmt.Printf("%q\n", PathSplitExt("dir/.rc"))
	fmt.Printf("%q\n", PathSplitExt("home/.secret/file"))
	// Output:
	// {"" ".cshrc"}
	// {"dir/a" ".ext"}
	// {"dir/" ".rc"}
	// {"home/.secret/file" ""}
}
