// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

func optionTable(w io.Writer, rows [][]string) {
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				tw.Write([]byte("\t"))
			}
			tw.Write([]byte(cell))
		}
		tw.Write([]byte("\n"))
	}
	tw.Flush()
}

func usageFor(fs *flag.FlagSet, usage string, extra string) func() {
	return func() {
		var b bytes.Buffer
		b.WriteString("Usage:\n  " + usage + "\n")

		var options [][]string
		fs.VisitAll(func(f *flag.Flag) {
			var opt = "  -" + f.Name

			if f.DefValue != "false" {
				opt += "=" + fmt.Sprintf(`"%s"`, f.DefValue)
			}
			options = append(options, []string{opt, f.Usage})
		})

		if len(options) > 0 {
			b.WriteString("\nOptions:\n")
			optionTable(&b, options)
		}

		fmt.Println(b.String())

		if len(extra) > 0 {
			fmt.Println(extra)
		}
	}
}
