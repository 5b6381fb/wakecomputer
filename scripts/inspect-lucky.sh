#!/bin/sh
echo "== lucky dir =="
ls -la /etc/config/lucky.daji 2>&1 | head -20
echo "== files =="
find /etc/config/lucky.daji -maxdepth 2 -type f 2>/dev/null | head -20
echo "== domains/ports =="
grep -rhoE '"(domains|domain|listen|listenPort|port)"[^,}]{0,80}' /etc/config/lucky.daji 2>/dev/null | head -40
echo "== wan =="
ifstatus wan 2>/dev/null | grep -E '"proto"|"up"' | head -4
echo "== firewall redirects =="
uci show firewall 2>/dev/null | grep -E 'redirect|dest_port' | head -20
echo "== lucky listen =="
netstat -lntp 2>/dev/null | grep lucky
