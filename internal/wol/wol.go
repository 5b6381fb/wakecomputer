package wol

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

type Result struct {
	Address string `json:"address"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
}

// MagicPacket builds the 102-byte Wake-on-LAN payload.
func MagicPacket(mac net.HardwareAddr) []byte {
	p := make([]byte, 6+16*len(mac))
	for i := 0; i < 6; i++ {
		p[i] = 0xFF
	}
	for i := 0; i < 16; i++ {
		copy(p[6+i*len(mac):], mac)
	}
	return p
}

func ParseMAC(s string) (net.HardwareAddr, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), "-", ":")
	mac, err := net.ParseMAC(s)
	if err != nil {
		return nil, fmt.Errorf("invalid MAC address")
	}
	if len(mac) != 6 {
		return nil, fmt.Errorf("only 48-bit MAC addresses are supported")
	}
	return mac, nil
}

// Send writes the magic packet to every target (host:port or bare host).
// All packets share one socket, which has SO_BROADCAST enabled on Linux.
func Send(mac net.HardwareAddr, targets []string, defaultPort int) ([]Result, error) {
	if defaultPort <= 0 {
		defaultPort = 9
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	enableBroadcast(conn)

	payload := MagicPacket(mac)
	seen := map[string]bool{}
	type target struct {
		raw  string
		addr *net.UDPAddr
		res  *Result
	}
	var list []*target
	var failures []Result

	for _, raw := range targets {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		addr, err := normalizeTarget(raw, defaultPort)
		if err != nil {
			failures = append(failures, Result{Address: raw, Error: err.Error()})
			continue
		}
		if seen[addr.String()] {
			continue
		}
		seen[addr.String()] = true
		list = append(list, &target{raw: raw, addr: addr, res: &Result{Address: addr.String(), OK: true}})
	}
	if len(list) == 0 && len(failures) == 0 {
		return nil, fmt.Errorf("no broadcast target configured")
	}

	// 3 rounds x 3 packets per target, 100 ms apart, matching the requirements.
	for round := 0; round < 3; round++ {
		for _, t := range list {
			for i := 0; i < 3; i++ {
				if _, err := conn.WriteToUDP(payload, t.addr); err != nil && t.res.OK {
					t.res.OK = false
					t.res.Error = err.Error()
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	results := failures
	for _, t := range list {
		results = append(results, *t.res)
	}
	return results, nil
}

func normalizeTarget(raw string, defaultPort int) (*net.UDPAddr, error) {
	host, portStr := raw, ""
	if strings.HasPrefix(raw, "[") {
		// IPv6 literal, not supported for broadcast but parse cleanly
		return nil, fmt.Errorf("IPv6 target not supported")
	}
	if strings.Count(raw, ":") == 1 {
		host, portStr, _ = strings.Cut(raw, ":")
	}
	port := defaultPort
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil || p <= 0 || p > 65535 {
			return nil, fmt.Errorf("invalid port %q", portStr)
		}
		port = p
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// allow hostnames (e.g. router DNS name) but they must resolve to IPv4
		ips, err := net.LookupIP(host)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", host, err)
		}
		for _, candidate := range ips {
			if v4 := candidate.To4(); v4 != nil {
				ip = v4
				break
			}
		}
		if ip == nil {
			return nil, fmt.Errorf("no IPv4 address for %s", host)
		}
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return &net.UDPAddr{IP: ip, Port: port}, nil
}
