# BlackChain Town v1 Host-Site Placement Checklist

Use this checklist on the target host before service start.

## Binary Placement

- `/usr/local/bin/blacknetd` present
- `/usr/local/bin/blackctl` present
- `/usr/local/bin/signtx` present if required

## Config Placement

- `/etc/blackchain/bootstrap/config.json` present
- `/etc/blackchain/node1/config.json` present
- `/etc/blackchain/node2/config.json` present
- `/etc/blackchain/node3/config.json` present
- optional `/etc/blackchain/<node>/blacknetd.env` files placed where intended

## Data Placement

- `/var/lib/blackchain/bootstrap` present
- `/var/lib/blackchain/node1` present
- `/var/lib/blackchain/node2` present
- `/var/lib/blackchain/node3` present

## TLS Placement

- `/etc/blackchain/tls/ca.pem` present
- `/etc/blackchain/tls/bootstrap/cert.pem` and `key.pem` present
- `/etc/blackchain/tls/node1/cert.pem` and `key.pem` present
- `/etc/blackchain/tls/node2/cert.pem` and `key.pem` present
- `/etc/blackchain/tls/node3/cert.pem` and `key.pem` present

## Service Placement

- `/etc/systemd/system/blacknetd@.service` present
- `systemctl daemon-reload` completed after unit placement

