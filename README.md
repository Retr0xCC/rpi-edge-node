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

## Project Structure

```text
rpi-edge-node/
├── ansible/
│   ├── ansible.cfg                    # Ansible runtime settings and role search path
│   ├── inventory.ini                  # Target host inventory
│   ├── site.yml                       # Primary deployment playbook
│   ├── group_vars/
│   │   └── all/
│   │       ├── vars.yml               # Public infrastructure & network variables
│   │       ├── vault.example.yml      # Template for encrypted secrets
│   │       └── vault.yml              # Ansible Vault encrypted credentials
│   └── roles/
│       ├── base/                      # System packages, sysctl, UFW firewall & DNS redirect
│       ├── singbox/                   # Transparent proxy, routing rules & DNS resolver
│       │   ├── templates/
│       │   │   └── config.json.j2     # sing-box runtime configuration
│       │   └── vars/
│       │       ├── vault_proton.example.yml  # Reference template for Proton WireGuard nodes
│       │       └── vault_proton.yml          # Encrypted Proton VPN endpoints
│       ├── amneziawg/                 # AmneziaWG VPN server endpoint
│       ├── docker/                    # Docker container runtime
│       └── minecraft/                 # Dedicated game server container
└── tools/
    └── proton-parser/                 # Go pre-processor for Proton VPN WireGuard configs
        ├── configs/                   # Git-ignored inbox for downloaded .conf files
        │   ├── <account1>/
        │   │   ├── <node-1>.conf
        │   │   └── <node-2>.conf
        │   └── <account2>/
        │       └── <node-3>.conf
        ├── go.mod
        ├── main.go
        └── main_test.go
```

## Quick Start

### Prerequisites

- **Control Machine:** Ansible `>= 2.15` and Python 3.
- **Target Node:** Raspberry Pi 4B (or compatible ARM64 SBC) running DietPi (Debian 12 Bookworm).
- **Access:** Working SSH key authentication with passwordless `sudo` privileges.

### Configuration

1. **Clone the repository and enter the Ansible directory:**
   ```bash
   git clone https://codeberg.org/Retr0defeated/rpi-edge-node.git
   cd rpi-edge-node/ansible
   ```

2. **Configure sensitive parameters:**
   ```bash
   cp group_vars/all/vault.example.yml group_vars/all/vault.yml
   ansible-vault encrypt group_vars/all/vault.yml
   ```

3. **Configure Proton VPN endpoints (Optional):**
   Generate `vault_proton.yml` automatically using the [Proton WireGuard Pipeline](#proton-wireguard-pipeline-toolsproton-parser), or copy the example template:
   ```bash
   cp roles/singbox/vars/vault_proton.example.yml roles/singbox/vars/vault_proton.yml
   ansible-vault encrypt roles/singbox/vars/vault_proton.yml
   ```

4. **Define network inventory and baseline variables:**
   Review and adjust `inventory.ini` (or `inventory/hosts.yml`) and `group_vars/all/vars.yml` with your local subnet ranges, upstream router IP, and preferred service ports.

### Deployment

Run the Ansible playbook against the target node:

```bash
ansible-playbook -i inventory.ini site.yml --ask-vault-pass
```

> If an inventory file such as `inventory/hosts.yml` is used, specify `-i inventory/hosts.yml` accordingly.

## Proton WireGuard Pipeline (`tools/proton-parser`)

The `tools/proton-parser` CLI automates extracting and transforming downloaded Proton VPN WireGuard `.conf` files into production-ready Ansible variables for the `singbox` role.

### Inbox Layout

Organize downloaded WireGuard `.conf` files by account inside `tools/proton-parser/configs/` (this directory is ignored by Git):

```text
tools/proton-parser/configs/
├── acc1/
│   ├── nl-101.conf
│   └── nl-102.conf
└── acc2/
    └── nl-201.conf
```

### Usage

Run the parser from the tool directory or repository root:

```bash
cd tools/proton-parser

# Parse configs and generate unencrypted vault_proton.yml
go run main.go

# Parse configs and automatically encrypt with Ansible Vault (auto-discovers .vault_pass)
go run main.go --encrypt

# Specify explicit password file or shorthand flags if needed
go run main.go -e -p /path/to/.vault_pass
```

CLI flags:
* `-i, --in`: Path to source configs directory (default: `./tools/proton-parser/configs` or `./configs`).
* `-o, --out`: Output YAML destination (default: `./ansible/roles/singbox/vars/vault_proton.yml`).
* `-e, --encrypt`: Encrypt the output file in-place with `ansible-vault encrypt` (automatically discovers `.vault_pass` across directory tree if omitted).
* `-p, --vault-pass-file, --vault-password-file`: Optional explicit path to the Ansible Vault password file.

### Configuration Schema (`vault_proton.yml`)

The generated YAML file conforms to the following structure:

```yaml
---
# Auto-generated by tools/proton-parser. DO NOT EDIT MANUALLY.
singbox_proton_accounts:
  - name: "acc1"
    group_tag: "proton-group-acc1"
    nodes:
      - tag: "proton-acc1-nl-101"
        server: "185.159.157.2"
        server_port: 51820
        local_address: "10.2.0.2/32"
        private_key: "PRIVATE_KEY_HERE"
        peer_public_key: "PUBLIC_KEY_HERE"
        mtu: 1280
      - tag: "proton-acc1-nl-102"
        server: "185.159.157.3"
        server_port: 51820
        local_address: "10.2.0.2/32"
        private_key: "PRIVATE_KEY_HERE"
        peer_public_key: "PUBLIC_KEY_HERE"
        mtu: 1280
  - name: "acc2"
    group_tag: "proton-group-acc2"
    nodes:
      - tag: "proton-acc2-nl-201"
        server: "185.159.157.4"
        server_port: 51820
        local_address: "10.2.0.3/32"
        private_key: "PRIVATE_KEY_HERE"
        peer_public_key: "PUBLIC_KEY_HERE"
        mtu: 1280
```

## Security & Secrets

- **Encrypted Secrets:** All sensitive credentials (VPN private keys, obfuscation parameters, storage passwords, and internal DNS entries) are strictly managed via **Ansible Vault**.
- **Log Leak Prevention:** Sensitive templates deploy with restrictive filesystem permissions (`mode: '0600'`) and `diff: false` to ensure confidential keys never leak into execution logs or CI stdout.
- **Filesystem Guardrails:** Storage mount targets are protected with immutable flags (`chattr +i`) when unmounted, guaranteeing local flash storage integrity during network or upstream NAS outages.

