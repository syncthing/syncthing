// Copyright (C) 2012 Numerotron Inc.
// Use of this source code is governed by an MIT-style license
// that can be found in the LICENSE file.

package stathat_test

import (
	"log"

	stathat "github.com/stathat/go"
)

func ExamplePostEZCountOne() {
	log.Printf("starting example")
	stathat.Verbose = true
	err := stathat.PostEZCountOne("go example test run", "nobody@stathat.com")
	if err != nil {
		log.Printf("error posting ez count one: %v", err)
		return
	}
	// If using this with a valid account, this could be helpful in a short script:
	/*
		ok := stathat.WaitUntilFinished(5 * time.Second)
		if ok {
			fmt.Println("ok")
		}
	*/
	// Output:
}
