#!/bin/sh
echo "== dns =="
nslookup acme-v02.api.letsencrypt.org 223.5.5.5 2>&1 | tail -8
echo "== ipv4 https letsencrypt =="
curl -4 -sS -I --max-time 12 https://acme-v02.api.letsencrypt.org/directory 2>&1 | head -8
echo "== ipv6 https letsencrypt =="
curl -6 -sS -I --max-time 12 https://acme-v02.api.letsencrypt.org/directory 2>&1 | head -8
echo "== baidu =="
curl -4 -sS -I --max-time 10 https://www.baidu.com 2>&1 | head -5
echo "== clash =="
ps w 2>/dev/null | grep -iE 'clash|mihomo' | grep -v grep | head -5
echo "== proxy env =="
env | grep -iE 'proxy' | head -5
echo "== route =="
ip route | head -4
