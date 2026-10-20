package discovery

import (
	"encoding/binary"
	"testing"
)

func TestParseSubnet(t *testing.T) {
	valid := []string{"192.168.1.0/24", "192.168.1.0", "10.0.0.0/28", "172.16.5.0/24"}
	for _, in := range valid {
		if _, err := ParseSubnet(in); err != nil {
			t.Errorf("ParseSubnet(%q): %v", in, err)
		}
	}
	invalid := []string{"8.8.8.0/24", "192.168.0.0/16", "192.168.1.0/32x", "not-a-subnet", "fd00::/64"}
	for _, in := range invalid {
		if _, err := ParseSubnet(in); err == nil {
			t.Errorf("ParseSubnet(%q) unexpectedly succeeded", in)
		}
	}
}

func TestHostIPs(t *testing.T) {
	sn, err := ParseSubnet("192.168.1.0/24")
	if err != nil {
		t.Fatal(err)
	}
	ips, err := hostIPs(sn)
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 254 {
		t.Fatalf("host count = %d, want 254", len(ips))
	}
	if ips[0].String() != "192.168.1.1" || ips[253].String() != "192.168.1.254" {
		t.Fatalf("range = %s..%s", ips[0], ips[253])
	}
}

func TestParseNBSTATName(t *testing.T) {
	pkt := make([]byte, 12)
	binary.BigEndian.PutUint16(pkt[6:8], 1) // ANCOUNT
	pkt = append(pkt, 0xC0, 0x0C)           // answer name pointer
	pkt = append(pkt, 0x00, 0x21)           // type NBSTAT
	pkt = append(pkt, 0x00, 0x01)           // class IN
	pkt = append(pkt, 0, 0, 0, 60)          // TTL
	name := "DESKTOP-ABC    "
	rdata := append([]byte{1}, []byte(name)...)
	rdata = append(rdata, 0x00)
	pkt = append(pkt, byte(len(rdata)>>8), byte(len(rdata)))
	pkt = append(pkt, rdata...)

	if got := parseNBSTATName(pkt); got != "DESKTOP-ABC" {
		t.Fatalf("name = %q", got)
	}
	if got := parseNBSTATName([]byte{1, 2, 3}); got != "" {
		t.Fatalf("short packet returned %q", got)
	}
}
