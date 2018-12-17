package procfs

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Namespace represents a single namespace of a process.
type Namespace struct {
	Type  string // Namespace type.
	Inode uint32 // Inode number of the namespace. If two processes are in the same namespace their inodes will match.
}

// Namespaces contains all of the namespaces that the process is contained in.
type Namespaces map[string]Namespace

// NewNamespaces reads from /proc/[pid/ns/* to get the namespaces of which the
// process is a member.
func (p Proc) NewNamespaces() (Namespaces, error) {
	d, err := os.Open(p.path("ns"))
	if err != nil {
		return nil, err
	}
	defer d.Close()

	names, err := d.Readdirnames(-1)
	if err != nil {
		return nil, fmt.Errorf("failed to read contents of ns dir: %v", err)
	}

	ns := make(Namespaces, len(names))
	for _, name := range names {
		target, err := os.Readlink(p.path("ns", name))
		if err != nil {
			return nil, err
		}

		fields := strings.SplitN(target, ":", 2)
		if len(fields) != 2 {
			return nil, fmt.Errorf("failed to parse namespace type and inode from '%v'", target)
		}

		typ := fields[0]
		inode, err := strconv.ParseUint(strings.Trim(fields[1], "[]"), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("failed to parse inode from '%v': %v", fields[1], err)
		}

		ns[name] = Namespace{typ, uint32(inode)}
	}

	return ns, nil
}
