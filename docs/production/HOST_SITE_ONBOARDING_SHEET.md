# BlackChain Host-Site Onboarding Sheet

Host or site name:

Primary operator:

Backup operator:

Environment:

## Site Details

- hostname:
- physical or logical location:
- maintenance window:
- local access method:

## Deployment Paths

- binary path: `/usr/local/bin`
- config path: `/etc/blackchain`
- data path: `/var/lib/blackchain`
- systemd unit path: `/etc/systemd/system/blacknetd@.service`

## Node Mapping

| Node | Listen | HTTPS | Config Path | Data Path | Notes |
| --- | --- | --- | --- | --- | --- |
| bootstrap | 127.0.0.1:7071 | 127.0.0.1:6069 | /etc/blackchain/bootstrap/config.json | /var/lib/blackchain/bootstrap |  |
| node1 | 127.0.0.1:7072 | 127.0.0.1:6060 | /etc/blackchain/node1/config.json | /var/lib/blackchain/node1 |  |
| node2 | 127.0.0.1:7073 | 127.0.0.1:6061 | /etc/blackchain/node2/config.json | /var/lib/blackchain/node2 |  |
| node3 | 127.0.0.1:7074 | 127.0.0.1:6062 | /etc/blackchain/node3/config.json | /var/lib/blackchain/node3 |  |

## Site Readiness

- CA and cert placement confirmed
- service unit placement confirmed
- provisioning helper run
- post-install smoke completed
- field verification completed
- signoff completed

