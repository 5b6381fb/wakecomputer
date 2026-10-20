package discovery

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type Device struct {
	IP       string `json:"ip"`
	MAC      string `json:"mac"`
	Hostname string `json:"hostname,omitempty"`
	Vendor   string `json:"vendor,omitempty"`
	Added    bool   `json:"added"`
}

type Result struct {
	Subnet    string   `json:"subnet"`
	ScannedAt string   `json:"scannedAt"`
	Items     []Device `json:"items"`
}

// Scan warms the neighbor table with a light UDP probe (no privileges needed)
// and then reads the kernel ARP table.
func Scan(ctx context.Context, subnet *net.IPNet, wait time.Duration, probe bool) ([]Device, error) {
	if subnet == nil {
		return nil, fmt.Errorf("subnet is required")
	}
	ips, err := hostIPs(subnet)
	if err != nil {
		return nil, err
	}
	if probe {
		probeHosts(ctx, ips)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	entries, err := arpEntries()
	if err != nil {
		return nil, err
	}
	var out []Device
	for _, d := range entries {
		ip := net.ParseIP(d.IP)
		if ip == nil || !subnet.Contains(ip) {
			continue
		}
		out = append(out, d)
	}

	// Resolve Windows workstation names in parallel; failures are fine.
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i := range out {
		i := i
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			out[i].Hostname = netbiosName(ctx, out[i].IP, 350*time.Millisecond)
		}()
	}
	wg.Wait()

	sort.Slice(out, func(i, j int) bool { return ipLess(out[i].IP, out[j].IP) })
	return out, nil
}

func hostIPs(subnet *net.IPNet) ([]net.IP, error) {
	ones, bits := subnet.Mask.Size()
	if bits != 32 {
		return nil, fmt.Errorf("only IPv4 subnets are supported")
	}
	if ones < 24 {
		return nil, fmt.Errorf("subnet is too large, /24 or smaller is required")
	}
	base := subnet.IP.To4()
	if base == nil {
		return nil, fmt.Errorf("invalid subnet")
	}
	count := 1 << (32 - ones)
	if count > 256 {
		return nil, fmt.Errorf("subnet is too large")
	}
	network := binary.BigEndian.Uint32(base)
	var out []net.IP
	for i := 1; i < count-1; i++ {
		v := network + uint32(i)
		ip := make(net.IP, 4)
		binary.BigEndian.PutUint32(ip, v)
		out = append(out, ip)
	}
	return out, nil
}

func probeHosts(ctx context.Context, ips []net.IP) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return
	}
	defer conn.Close()
	payload := []byte{0}
	for _, ip := range ips {
		select {
		case <-ctx.Done():
			return
		default:
		}
		// A single datagram makes the kernel resolve the neighbor entry.
		_, _ = conn.WriteToUDP(payload, &net.UDPAddr{IP: ip, Port: 9})
	}
}

func arpEntries() ([]Device, error) {
	raw, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		return nil, fmt.Errorf("read arp table: %w", err)
	}
	lines := strings.Split(string(raw), "\n")
	var out []Device
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		ip := fields[0]
		flags := fields[2]
		mac := strings.ToLower(fields[3])
		if flags == "0x0" || mac == "00:00:00:00:00:00" {
			continue
		}
		if net.ParseIP(ip) == nil || net.ParseIP(ip).To4() == nil {
			continue
		}
		out = append(out, Device{IP: ip, MAC: mac})
	}
	return out, nil
}

func ipLess(a, b string) bool {
	ia := net.ParseIP(a).To4()
	ib := net.ParseIP(b).To4()
	if ia == nil || ib == nil {
		return a < b
	}
	return binary.BigEndian.Uint32(ia) < binary.BigEndian.Uint32(ib)
}

// netbiosName performs a NetBIOS node status query (NBSTAT) to get the
// Windows workstation name. Non-Windows hosts simply return "".
func netbiosName(ctx context.Context, ip string, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	d := net.Dialer{Timeout: timeout, Deadline: deadline}
	conn, err := d.DialContext(ctx, "udp4", net.JoinHostPort(ip, "137"))
	if err != nil {
		return ""
	}
	defer conn.Close()
	_ = conn.SetDeadline(deadline)

	query := buildNBSTAT()
	if _, err := conn.Write(query); err != nil {
		return ""
	}
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil || n < 12 {
		return ""
	}
	return parseNBSTATName(buf[:n])
}

func buildNBSTAT() []byte {
	// Header: id, flags, qd=1, an=0, ns=0, ar=0
	p := []byte{0x12, 0x34, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	// Encoded wildcard name "*": 0x20 + "CK" + 30 "A".
	p = append(p, 0x20, 'C', 'K')
	for i := 0; i < 30; i++ {
		p = append(p, 'A')
	}
	p = append(p, 0x00)       // root label
	p = append(p, 0x00, 0x21) // QTYPE NBSTAT
	p = append(p, 0x00, 0x01) // QCLASS IN
	return p
}

func parseNBSTATName(pkt []byte) string {
	if len(pkt) < 12 {
		return ""
	}
	qd := int(binary.BigEndian.Uint16(pkt[4:6]))
	an := int(binary.BigEndian.Uint16(pkt[6:8]))
	off := 12
	// Skip questions.
	for i := 0; i < qd; i++ {
		for {
			if off >= len(pkt) {
				return ""
			}
			l := int(pkt[off])
			off++
			if l == 0 {
				break
			}
			if l&0xC0 == 0xC0 {
				off++
				break
			}
			off += l
		}
		off += 4
	}
	if an == 0 || off+12 > len(pkt) {
		return ""
	}
	// Answer name (usually a 0xC0 pointer).
	if pkt[off]&0xC0 == 0xC0 {
		off += 2
	} else {
		for off < len(pkt) && pkt[off] != 0 {
			off += int(pkt[off]) + 1
		}
		off++
	}
	off += 8 // type(2) class(2) ttl(4)
	if off+2 > len(pkt) {
		return ""
	}
	rdlen := int(binary.BigEndian.Uint16(pkt[off : off+2]))
	off += 2
	if rdlen < 1 || off >= len(pkt) || off+rdlen > len(pkt) {
		return ""
	}
	num := int(pkt[off])
	off++
	if num < 1 || off+15 > len(pkt) {
		return ""
	}
	name := strings.TrimSpace(string(pkt[off : off+15]))
	if name == "" || strings.ContainsAny(name, "\x00") {
		return ""
	}
	return name
}

// ParseSubnet validates a user-supplied private IPv4 subnet.
func ParseSubnet(raw string) (*net.IPNet, error) {
	if !strings.Contains(raw, "/") {
		raw = raw + "/24"
	}
	ip, network, err := net.ParseCIDR(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid subnet")
	}
	if ip.To4() == nil {
		return nil, fmt.Errorf("only IPv4 is supported")
	}
	ones, bits := network.Mask.Size()
	if bits != 32 || ones < 24 {
		return nil, fmt.Errorf("subnet must be /24 or smaller")
	}
	if !ip.IsPrivate() {
		return nil, fmt.Errorf("only private subnets are allowed")
	}
	return network, nil
}

func DefaultSubnet(broadcast string) string {
	if ip := net.ParseIP(strings.TrimSpace(broadcast)); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return fmt.Sprintf("%d.%d.%d.0/24", v4[0], v4[1], v4[2])
		}
	}
	return "192.168.1.0/24"
}
