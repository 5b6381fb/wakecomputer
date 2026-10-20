#!/bin/sh
echo "== arp =="
head -12 /proc/net/arp
echo "== dhcp leases =="
head -8 /tmp/dhcp.leases 2>/dev/null || echo no-leases
echo "== reverse dns =="
for ip in "$@"; do
  echo -n "$ip -> "
  nslookup "$ip" 127.0.0.1 2>/dev/null | grep -E 'name =' | head -1 || echo none
done
