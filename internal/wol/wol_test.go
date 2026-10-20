package wol

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func TestMagicPacket(t *testing.T) {
	mac, err := ParseMAC("AA-BB-CC-DD-EE-FF")
	if err != nil {
		t.Fatal(err)
	}
	p := MagicPacket(mac)
	if len(p) != 102 {
		t.Fatalf("len = %d, want 102", len(p))
	}
	if !bytes.Equal(p[:6], []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}) {
		t.Fatalf("header = %x", p[:6])
	}
	for i := 0; i < 16; i++ {
		if !bytes.Equal(p[6+i*6:12+i*6], mac) {
			t.Fatalf("mac repetition %d mismatch", i)
		}
	}
}

func TestParseMAC(t *testing.T) {
	for _, in := range []string{"AA:BB:CC:DD:EE:FF", "aa-bb-cc-dd-ee-ff", " aa:bb:cc:dd:ee:ff "} {
		if _, err := ParseMAC(in); err != nil {
			t.Errorf("ParseMAC(%q): %v", in, err)
		}
	}
	for _, in := range []string{"", "aa:bb:cc", "zz:bb:cc:dd:ee:ff"} {
		if _, err := ParseMAC(in); err == nil {
			t.Errorf("ParseMAC(%q) unexpectedly succeeded", in)
		}
	}
}

func TestSendToLoopback(t *testing.T) {
	ln, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Skipf("cannot listen: %v", err)
	}
	defer ln.Close()
	port := ln.LocalAddr().(*net.UDPAddr).Port

	mac, _ := ParseMAC("AA:BB:CC:DD:EE:FF")
	results, err := Send(mac, []string{net.JoinHostPort("127.0.0.1", itoa(port))}, 9)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].OK {
		t.Fatalf("unexpected results: %+v", results)
	}

	_ = ln.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 2048)
	seen := 0
	for {
		n, _, err := ln.ReadFromUDP(buf)
		if err != nil {
			break
		}
		if n == 102 && bytes.Equal(buf[:6], []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}) {
			seen++
		}
		if seen >= 9 {
			break
		}
	}
	if seen != 9 {
		t.Fatalf("received %d magic packets, want 9", seen)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
