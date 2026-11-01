//go:build with_utls

// Copyright (C) 2022 nekohasekai. Derived from sing-vmess v0.2.7.
// SPDX-License-Identifier: GPL-3.0-or-later
package vision

import (
	"net"
	"reflect"
	"unsafe"

	N "github.com/sagernet/sing/common/network"

	utls "github.com/metacubex/utls"
)

func init() {
	tlsRegistry = append(tlsRegistry, func(conn net.Conn) (loaded bool, netConn net.Conn, reflectType reflect.Type, reflectPointer unsafe.Pointer) {
		uConn, loaded := N.CastReader[*utls.UConn](conn)
		if loaded {
			return true, uConn.NetConn(), reflect.TypeOf(uConn.Conn).Elem(), unsafe.Pointer(uConn.Conn)
		}
		tlsConn, loaded := N.CastReader[*utls.Conn](conn)
		if loaded {
			return true, tlsConn.NetConn(), reflect.TypeOf(tlsConn).Elem(), unsafe.Pointer(tlsConn)
		}
		return
	})
}
