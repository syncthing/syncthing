package missinggo

import (
	"os"
	"path"
)

// Splits the pathname p into Root and Ext, such that Root+Ext==p.
func PathSplitExt(p string) (ret struct {
	Root, Ext string
}) {
	ret.Ext = path.Ext(p)
	ret.Root = p[:len(p)-len(ret.Ext)]
	return
}

func FilePathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
