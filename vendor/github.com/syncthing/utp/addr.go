package utp

import "net"

type addr struct {
	socket net.Addr
}

func (me addr) Network() string {
	return "utp/" + me.socket.Network()
}

func (me addr) String() string {
	return me.socket.String()
}
