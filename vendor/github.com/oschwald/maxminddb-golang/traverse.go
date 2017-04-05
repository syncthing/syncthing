package maxminddb

import "net"

// Internal structure used to keep track of nodes we still need to visit.
type netNode struct {
	ip      net.IP
	bit     uint
	pointer uint
}

// Networks represents a set of subnets that we are iterating over.
type Networks struct {
	reader   *Reader
	nodes    []netNode // Nodes we still have to visit.
	lastNode netNode
	err      error
}

// Networks returns an iterator that can be used to traverse all networks in
// the database.
//
// Please note that a MaxMind DB may map IPv4 networks into several locations
// in in an IPv6 database. This iterator will iterate over all of these
// locations separately.
func (r *Reader) Networks() *Networks {
	s := 4
	if r.Metadata.IPVersion == 6 {
		s = 16
	}
	return &Networks{
		reader: r,
		nodes: []netNode{
			{
				ip: make(net.IP, s),
			},
		},
	}
}

// Next prepares the next network for reading with the Network method. It
// returns true if there is another network to be processed and false if there
// are no more networks or if there is an error.
func (n *Networks) Next() bool {
	for len(n.nodes) > 0 {
		node := n.nodes[len(n.nodes)-1]
		n.nodes = n.nodes[:len(n.nodes)-1]

		for {
			if node.pointer < n.reader.Metadata.NodeCount {
				ipRight := make(net.IP, len(node.ip))
				copy(ipRight, node.ip)
				if len(ipRight) <= int(node.bit>>3) {
					n.err = newInvalidDatabaseError(
						"invalid search tree at %v/%v", ipRight, node.bit)
					return false
				}
				ipRight[node.bit>>3] |= 1 << (7 - (node.bit % 8))

				rightPointer, err := n.reader.readNode(node.pointer, 1)
				if err != nil {
					n.err = err
					return false
				}

				node.bit++
				n.nodes = append(n.nodes, netNode{
					pointer: rightPointer,
					ip:      ipRight,
					bit:     node.bit,
				})

				node.pointer, err = n.reader.readNode(node.pointer, 0)
				if err != nil {
					n.err = err
					return false
				}

			} else if node.pointer > n.reader.Metadata.NodeCount {
				n.lastNode = node
				return true
			} else {
				break
			}
		}
	}

	return false
}

// Network returns the current network or an error if there is a problem
// decoding the data for the network. It takes a pointer to a result value to
// decode the network's data into.
func (n *Networks) Network(result interface{}) (*net.IPNet, error) {
	if err := n.reader.retrieveData(n.lastNode.pointer, result); err != nil {
		return nil, err
	}

	return &net.IPNet{
		IP:   n.lastNode.ip,
		Mask: net.CIDRMask(int(n.lastNode.bit), len(n.lastNode.ip)*8),
	}, nil
}

// Err returns an error, if any, that was encountered during iteration.
func (n *Networks) Err() error {
	return n.err
}
