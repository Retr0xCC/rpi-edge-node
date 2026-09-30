**English** | [Русский](README.ru.md)

# rpi-edge-node

> Automated Home Edge Gateway with sing-box & Ansible

[![Ansible](https://img.shields.io/badge/Ansible-2.15+-EE0000?style=flat-square&logo=ansible&logoColor=white)](https://www.ansible.com/)
[![sing-box](https://img.shields.io/badge/sing--box-1.9+-000000?style=flat-square)](https://sing-box.sagernet.org/)
[![Platform](https://img.shields.io/badge/DietPi-Debian%20Bookworm-blue?style=flat-square&logo=debian)](https://dietpi.com/)
[![Architecture](https://img.shields.io/badge/Arch-ARM64-brightgreen?style=flat-square&logo=arm)](https://www.raspberrypi.com/)
[![Security](https://img.shields.io/badge/Security-Ansible%20Vault-critical?style=flat-square)](https://docs.ansible.com/ansible/latest/vault_guide/index.html)

## Overview

**rpi-edge-node** automates the deployment and configuration of a dedicated home L3 edge gateway on a Raspberry Pi running DietPi (Debian Bookworm ARM64). It orchestrates sing-box in TUN mode, an AmneziaWG VPN server for remote client access, dual-stack firewalling (UFW), and DNS interception to provide transparent routing, circumvention, and local network integration across client devices without requiring per-client proxy software.

## Key Features

- **Transparent Proxying via sing-box TUN:** Captures transit network traffic using an L3 TUN adapter with strict RFC 1918 / ULA bypass (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `fd00::/8`, `fe80::/10`) for direct access to local services and reverse proxies.
- **Full DNS Hijacking (IPv4/IPv6):** UFW `PREROUTING` NAT rules redirect transit port 53 UDP/TCP traffic into the local sing-box resolver, preventing DNS leaks and intercepting hardcoded client DNS configurations.
- **Hardened Dual-Stack Networking:** Kernel sysctl configured with loose reverse path filtering (`rp_filter = 2`) to eliminate asymmetric routing drops across multi-homed interfaces (`eth0`, `tun0`, `awg0`), alongside TCP MSS clamping to prevent PMTU black holes.
- **WireGuard / AmneziaWG Tunnel Integration:** High-performance kernel-level VPN endpoint with protocol obfuscation for remote clients connecting into the home network.
- **Secure & Idempotent SMB/CIFS Storage Mounting:** Remote storage automounted via `systemd-automount` with immutable attribute protection (`chattr +i`) on the mount point directory to prevent local disk writes when the NAS is unreachable.

## Network Architecture

```text
   [ LAN Clients ]                         [ Remote Clients ]
(Workstations, Mobile, IoT)              (Connecting via AmneziaWG)
             │                                       │
             │ (eth0)                                │ (awg0)
             ▼                                       ▼
  ┌─────────────────────────────────────────────────────────────┐
  │                  Raspberry Pi Edge Node                     │
  │                                                             │
  │   Ingress Interfaces (eth0 / awg0)                          │
  │        │                                                    │
  │   UFW Firewall (NAT PREROUTING: DNS Hijack :53)             │
  │        │                                                    │
  │   sing-box Core (tun0 & Routing Engine)                     │
  │        ├── Local / Direct Traffic ─────────────┐            │
  │        └── Proxied / Target Domains ───────┐   │            │
  └────────────────────────────────────────────│───│────────────┘
                                               │   │
                   ┌───────────────────────────┘   │
                   │ (Encrypted Proxy Tunnel)      │ (Direct Routing)
                   ▼                               ▼
          [ Remote VPS Server ]           [ Upstream Router ]
        (Hysteria2 / Outbound)            (Physical Gateway / ISP)
                   │                               │
                   └───────────────┬───────────────┘
                                   ▼
                             [ Internet ]
```

## Quick Start

### Prerequisites

- **Control Machine:** Ansible `>= 2.15` and Python 3.
- **Target Node:** Raspberry Pi 4B (or compatible ARM64 SBC) running DietPi (Debian 12 Bookworm).
- **Access:** Working SSH key authentication with passwordless `sudo` privileges.

### Configuration

1. **Clone the repository and enter the Ansible directory:**
   ```bash
   git clone https://github.com/example/rpi-edge-node.git
   cd rpi-edge-node/ansible
   ```

2. **Configure sensitive parameters:**
   ```bash
   cp group_vars/all/vault.example.yml group_vars/all/vault.yml
   ansible-vault encrypt group_vars/all/vault.yml
   ```

3. **Define network inventory and baseline variables:**
   Review and adjust `inventory.ini` (or `inventory/hosts.yml`) and `group_vars/all/vars.yml` with your local subnet ranges, upstream router IP, and preferred service ports.

### Deployment

Run the Ansible playbook against the target node:

```bash
ansible-playbook -i inventory.ini site.yml --ask-vault-pass
```

> If an inventory file such as `inventory/hosts.yml` is used, specify `-i inventory/hosts.yml` accordingly.

## Security & Secrets

- **Encrypted Secrets:** All sensitive credentials (VPN private keys, obfuscation parameters, storage passwords, and internal DNS entries) are strictly managed via **Ansible Vault**.
- **Log Leak Prevention:** Sensitive templates deploy with restrictive filesystem permissions (`mode: '0600'`) and `diff: false` to ensure confidential keys never leak into execution logs or CI stdout.
- **Filesystem Guardrails:** Storage mount targets are protected with immutable flags (`chattr +i`) when unmounted, guaranteeing local flash storage integrity during network or upstream NAS outages.
