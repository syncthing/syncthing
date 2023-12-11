// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"fmt"
	"os"
	"text/tabwriter"
)

// indexAccount prints key and data size statistics per class
func indexAccount() error {
	ldb, err := getDB()
	if err != nil {
		return err
	}

	it, err := ldb.NewPrefixIterator(nil)
	if err != nil {
		return err
	}

	var ksizes [256]int
	var dsizes [256]int
	var counts [256]int
	var max [256]int

	for it.Next() {
		key := it.Key()
		t := key[0]
		ds := len(it.Value())
		ks := len(key)
		s := ks + ds

		counts[t]++
		ksizes[t] += ks
		dsizes[t] += ds
		if s > max[t] {
			max[t] = s
		}
	}

	tw := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', tabwriter.AlignRight)
	toti, totds, totks := 0, 0, 0
	for t := range ksizes {
		if ksizes[t] > 0 {
			// yes metric kilobytes 🤘
			fmt.Fprintf(tw, "0x%02x:\t%d items,\t%d KB keys +\t%d KB data,\t%d B +\t%d B avg,\t%d B max\t\n", t, counts[t], ksizes[t]/1000, dsizes[t]/1000, ksizes[t]/counts[t], dsizes[t]/counts[t], max[t])
			toti += counts[t]
			totds += dsizes[t]
			totks += ksizes[t]
		}
	}
	fmt.Fprintf(tw, "Total\t%d items,\t%d KB keys +\t%d KB data.\t\n", toti, totks/1000, totds/1000)
	tw.Flush()

	return nil
}
