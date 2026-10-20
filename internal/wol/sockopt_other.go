//go:build !linux && !windows

package wol

import "net"

func enableBroadcast(*net.UDPConn) {}
