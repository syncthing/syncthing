#!/bin/bash

cat <<EOT
package auto

import "compress/gzip"
import "bytes"
import "io/ioutil"

var Assets = make(map[string][]byte)

func init() {
	var data []byte
	var gr *gzip.Reader
EOT

cd gui
for f in $(find . -type f) ; do 
	f="${f#./}"
	echo "gr, _ = gzip.NewReader(bytes.NewBuffer([]byte{"
	gzip -c $f | od -vt x1 | sed 's/^[0-9a-f]*//' | sed 's/\([0-9a-f][0-9a-f]\)/0x\1,/g'
	echo "}))"
	echo "data, _ = ioutil.ReadAll(gr)"
	echo "Assets[\"$f\"] = data"
done
echo "}"

