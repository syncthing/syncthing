// +build appengine

package maxminddb

func (d *decoder) decodeStructKey(offset uint) (string, uint, error) {
	return d.decodeKeyString(offset)
}
