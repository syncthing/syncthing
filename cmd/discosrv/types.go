// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).

package main

type address struct {
	ip   []byte
	port uint16
	seen int64 // epoch seconds
}

type addressList struct {
	addresses []address
}
