//go:build !with_utls

package vless

import (
	"errors"
	"net"
	"net/url"
)

const realityAvailable = false

func realityHandshake(conn net.Conn, q url.Values) (net.Conn, error) {
	return nil, errors.New("REALITY requires a with_utls build")
}
