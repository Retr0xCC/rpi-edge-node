**English** | [Русский](README.ru.md)

# rpi-edge-node

> Automated Home L3 Edge Gateway on Raspberry Pi with Smart Routing via sing-box and Ansible

[![Ansible](https://img.shields.io/badge/Ansible-2.15+-EE0000.svg?logo=ansible&logoColor=white)](https://www.ansible.com/)
[![sing-box](https://img.shields.io/badge/sing--box-1.10+-black.svg)](https://sing-box.sagernet.org/)
[![DietPi](https://img.shields.io/badge/DietPi-Debian%20Bookworm-blue.svg?logo=debian)](https://dietpi.com/)
[![Platform](https://img.shields.io/badge/Platform-ARM64%20%7C%20RPi-brightgreen.svg?logo=raspberrypi&logoColor=white)](https://www.raspberrypi.com/)
[![Security: Vault](https://img.shields.io/badge/Security-Ansible%20Vault-red.svg)](https://docs.ansible.com/ansible/latest/vault_guide/index.html)
[![Codeberg](https://img.shields.io/badge/Codeberg-Repository-2185D0?logo=codeberg&logoColor=white)](https://codeberg.org/Retr0defeated/rpi-edge-node)

---

## Overview

When a home network has many devices — workstations, laptops, phones, smart TVs, streaming boxes, and consoles — configuring proxy and VPN apps on each individual device is tedious, and often impossible.

**rpi-edge-node** transforms a compact Raspberry Pi running DietPi (Debian 12 Bookworm ARM64) into a centralized, transparent edge gateway. Simply set the Raspberry Pi's IP address as your default gateway on your home router or clients, and all traffic routing happens automatically:

* **Smart Traffic Splitting:** Local services, banking, and domestic websites route directly through your ISP at full line speed. Blocked services go through a fast, encrypted Hysteria 2 tunnel, Google and Gemini services route through Proton VPN, while Spotify and specialized forums route via Cloudflare WARP (MASQUE).
* **Leak-Proof DNS Interception:** All DNS requests are intercepted at the firewall level. Even if a smart TV or IoT device has hardcoded `8.8.8.8` DNS servers, the gateway redirects queries to the local sing-box resolver, preventing ISP DNS hijacking and leaks.
* **Remote Access via AmneziaWG:** Connect back to your home network securely from anywhere in the world using kernel-level WireGuard enhanced with DPI obfuscation (junk packet padding and randomized handshakes).
* **Resilient SMB/CIFS Network Storage:** Network shares automount on-demand via `systemd-automount` with local filesystem write protection (`chattr +i`) to prevent disk fill-ups when the NAS is offline.
* **One-Click Deployment:** The entire system — kernel parameters, packages, routing rules, firewall tables, and systemd units — is fully automated and reproducible via Ansible.

---

## Architecture & Routing Model

```text
 ┌─────────────────────────────────────────────────────────────┐
 │                           Clients                           │
 │                                                             │
 │   - Local LAN: PCs, Phones, Smart TVs, Consoles             │
 │   - Remote Clients: Mobile devices and laptops outside home │
 └──────────────┬──────────────────────────────┬───────────────┘
                │                              │
                │ Ethernet (eth0)              │ AmneziaWG (awg0)
                ▼                              ▼
 ┌─────────────────────────────────────────────────────────────┐
 │                  Raspberry Pi Edge Gateway                  │
 │                                                             │
 │   - Linux Kernel: IP Forwarding, BBR, rp_filter = 2, MSS Cl.│
 │   - UFW Firewall: NAT PREROUTING (DNS :53 Interception)     │
 │   - sing-box DNS: Dual-stack resolver, optimistic cache     │
 │   - sing-box TUN Routing (tun0):                            │
 │       ├── Private Subnets (RFC 1918) ─────► Direct          │
 │       ├── Ad Domains (geosite-ads) ───────► Reject (Drop)   │
 │       ├── ntc.party, Spotify ─────────────► direct-warp     │
 │       ├── Google & Gemini ────────────────► proton-group-acc1
 │       ├── Blocked / Telegram / Ru-bundle ─► proxy (Hy2)     │
 │       └── General Internet Traffic ───────► Direct          │
 └──────────────────────────────┬──────────────────────────────┘
                                │
                                │ Physical Uplink (eth0)
                                ▼
 ┌─────────────────────────────────────────────────────────────┐
 │             Home Router (Default Gateway / ISP)             │
 └──────────────┬──────────────────────────────┬───────────────┘
                │                              │
                │ Direct Traffic               │ Encrypted Tunnels
                │ (Bypasses VPS / VPN)         │
                │                              ▼
                │               ┌──────────────────────────────┐
                │               │     Upstream Nodes & VPN     │
                │               │                              │
                │               │ ├── Cloudflare Edge (WARP)   │
                │               │ ├── Remote VPS (Hysteria 2)  │
                │               │ └── Proton VPN (WireGuard)   │
                │               └──────────────┬───────────────┘
                │                              │
                ▼                              ▼
 ┌─────────────────────────────────────────────────────────────┐
 │                          Internet                           │
 └─────────────────────────────────────────────────────────────┘
```

### Key Mechanisms

* **Transparent Capture via TUN (sing-box):** Inbound transit traffic is captured on the `tun0` interface. RFC 1918 private subnets (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`) and IPv6 ULA (`fd00::/8`) are explicitly bypassed (`route_exclude_address`), preserving direct connectivity to LAN servers, printers, and reverse proxies.
* **DNS Hijacking & Clean Dual-Stack:** Conflicting `systemd-resolved` is masked and disabled permanently. All transit UDP/TCP port 53 traffic is redirected into sing-box via UFW `PREROUTING` rules. The DNS `strategy` is unconstrained, allowing full dual-stack operation required by services like `ntc.party`.
* **Kernel Sysctl Hardening:** Loose reverse path filtering (`rp_filter = 2`) is enforced across all interfaces (`eth0`, `tun0`, `awg0`), preventing asymmetric routing packet drops. TCP MSS Clamping ensures packets traverse VPN tunnels without PMTU blackholes.
* **Proton VPN Outbounds & Anti-Flapping:**
  Each Proton account operates as an isolated `urltest` failover group (`proton-group-<account>`). Google traffic is directed to the primary account. To avoid unnecessary reconnections:
  * `url`: Lightweight probe endpoint `https://cp.cloudflare.com/generate_204`.
  * `interval`: Tested every 10 minutes to avoid channel saturation.
  * `tolerance`: 50 ms latency tolerance buffer (switches only on genuine link degradation).
  * `idle_timeout`: 30 minutes (suspends health checks when traffic is idle).
  * Graceful fallback: defaults to `proxy` if no Proton accounts are configured.
* **Obfuscated AmneziaWG VPN:** An in-kernel WireGuard endpoint (`awg0`) accepts remote client connections. Traffic is protected with junk packet insertion and handshake obfuscation (`Jc`, `Jmin`, `Jmax`, `S1`–`S4`, `H1`–`H4`) to bypass mobile carrier DPI blocks.
* **Safe SMB/CIFS Automounting:** Remote storage at `/storage/nas` is managed by `systemd-automount`. When unmounted, the directory has the immutable attribute set (`chattr +i`), ensuring the SD card is never accidentally written to if the NAS is unreachable.

---

## Quick Start: Gateway Deployment

### Prerequisites

* **Control Machine:** Linux or macOS with Ansible `>= 2.15` and Python 3.
* **Target Hardware:** Raspberry Pi 4B (or compatible ARM64 single-board computer) with a clean [DietPi](https://dietpi.com/) (Debian 12 Bookworm) installation.
* **Network Access:** Working SSH key access with passwordless `sudo` privileges for user `dietpi` (or `root`).

### Step-by-Step Installation

#### 1. Clone the repository:

```bash
git clone https://codeberg.org/Retr0defeated/rpi-edge-node.git
cd rpi-edge-node/ansible
```

#### 2. Create the secrets file (`vault.yml`):

Copy the example template and fill in your credentials (VPS passwords, VPN keys, NAS credentials):

```bash
cp group_vars/all/vault.example.yml group_vars/all/vault.yml

# Encrypt the file using Ansible Vault:
ansible-vault encrypt group_vars/all/vault.yml
```

> [!TIP]
> To avoid entering the password on every playbook run, store your vault passphrase in `ansible/.vault_pass` (git-ignored) and configure it in `ansible.cfg` or supply `--vault-password-file .vault_pass`.

#### 3. Configure Proton VPN Nodes (Optional):

If you use Proton VPN WireGuard configurations:
1. Place downloaded WireGuard `.conf` files into subfolders under `tools/proton-parser/configs/`.
2. Generate the encrypted variables file:
   ```bash
   cd ../tools/proton-parser
   go run main.go --encrypt
   cd ../ansible
   ```
   This automatically generates `roles/singbox/vars/vault_proton.yml`.

#### 4. Configure inventory and network addresses:

Set your Raspberry Pi's IP address or SSH alias in `inventory.ini`:

```ini
[gateway]
rpi-node ansible_host=192.168.1.20 ansible_user=dietpi
```

Check the non-sensitive configuration in `group_vars/all/vars.yml` (ports, subnets, timezone).

#### 5. Run the deployment playbook:

```bash
ansible-playbook -i inventory.ini site.yml --ask-vault-pass
```

Once the playbook completes, all services start immediately without requiring a reboot.

---

## Configuration & Variables

Configuration is organized into public infrastructure variables and encrypted secrets:

### Public Variables (`ansible/group_vars/all/vars.yml`)

| Variable | Default | Description |
| :--- | :--- | :--- |
| `system_timezone` | `"Europe/Moscow"` | Host system timezone. |
| `smb_mount_enabled` | `true` | Enable SMB/CIFS automounting. |
| `smb_mount_point` | `"/storage/nas"` | Local mount point directory on the Raspberry Pi. |
| `ssh_port` | `22` | OpenSSH daemon listening port. |
| `singbox_dns_remote` | `"9.9.9.9"` | Public upstream resolver for proxied domains. |
| `awg_interface_name` | `"awg0"` | Interface name for the AmneziaWG VPN server. |
| `awg_listen_port` | `19302` | Inbound UDP port for AmneziaWG clients. |
| `awg_subnet_cidr` | `"10.9.9.1/24"` | Internal IPv4 subnet assigned to VPN clients. |
| `awg_ipv6_subnet_cidr` | `"fd00:9:9::1/64"` | Internal IPv6 subnet assigned to VPN clients. |
| `mc_service_dir` | `"/opt/minecraft"` | Base directory for containerized Minecraft server. |

### Secret Variables (`ansible/group_vars/all/vault.yml`)

Populated from `vault.example.yml`:
* `vault_smb_server_ip`, `vault_smb_share_name`, `vault_smb_username`, `vault_smb_password` — NAS credentials.
* `vault_primary_dns_server` — Gateway DNS address for `/etc/resolv.conf`.
* `vault_singbox_dns_local` — Local network DNS for internal domain resolution (`.lan`, `.local`, `.home`).
* `vault_singbox_hy2_server`, `vault_singbox_hy2_password`, `vault_singbox_hy2_obfs_password` — Primary Hysteria 2 proxy server settings.
* `vault_singbox_warp_*` — Cloudflare MASQUE (WARP) credentials and endpoints.
* `vault_awg_server_private_key`, `vault_awg_jmin`, `vault_awg_h1`... — AmneziaWG obfuscation parameters and private keys.

---

## Proton WireGuard Pipeline (`tools/proton-parser`)

The Go utility extracts endpoints from official Proton VPN `.conf` files and formats them into encrypted Ansible Vault dictionaries.

### Folder Structure

Place WireGuard `.conf` files into account directories inside `configs/` (git-ignored):

```text
tools/proton-parser/configs/
├── account1/
│   ├── wg-NL-FREE-101.conf
│   └── wg-NL-FREE-102.conf
└── account2/
    ├── wg-NL-FREE-201.conf
    └── wg-NL-FREE-202.conf
```

### CLI Commands

```bash
cd tools/proton-parser

# 1. Plain YAML generation (for dry-run inspection):
go run main.go

# 2. Parse and auto-encrypt with discovered .vault_pass:
go run main.go --encrypt

# 3. Explicit input/output paths and password file:
go run main.go -i ./configs -o ../../ansible/roles/singbox/vars/vault_proton.yml -e -p /path/to/.vault_pass
```

#### CLI Options

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--in` | `-i` | `./tools/proton-parser/configs` | Path to source directory containing WireGuard `.conf` files. |
| `--out` | `-o` | `./ansible/roles/singbox/vars/vault_proton.yml` | Output path for the generated YAML file. |
| `--encrypt` | `-e` | `false` | Automatically encrypt the generated file with `ansible-vault`. |
| `--vault-pass-file` | `-p` | `""` | Optional path to Ansible Vault password file. |

---

## Verification & Troubleshooting

After deployment, connect to your Raspberry Pi via SSH and run these quick checks:

```bash
# 1. Check network interfaces:
ip addr show tun0       # sing-box TUN proxy interface
ip addr show awg0       # AmneziaWG server interface

# 2. Check service status:
systemctl status sing-box

# 3. View live routing logs:
journalctl -u sing-box -f -n 50

# 4. Verify DNS interception:
dig @127.0.0.1 ntc.party
dig @127.0.0.1 google.com

# 5. Check outbound public IP:
# Direct connection (ISP):
curl -s https://api.ipify.org
# Test proxy routing:
curl -I https://www.google.com

# 6. Verify network storage automount:
ls -la /storage/nas
```

---

## Security & Resilience

- **Ansible Vault & Secret Masking:** Passwords and private keys are encrypted at rest. Template deployment tasks use `mode: '0600'` and `diff: false` to prevent credential exposure in logs.
- **Immutable Mount Point Protection:** The local directory `/storage/nas` is protected with `chattr +i`. If the NAS is disconnected, writing to the underlying SD card is prevented.
- **Fail-Safe Routing Fallback:** If Proton VPN account configurations are missing, routes gracefully fall back to the primary `proxy` outbound without syntax or runtime failures.
- **Anti-Flapping Protection:** A 50 ms latency tolerance buffer and 10-minute check intervals eliminate erratic IP switching caused by minor network jitter.

---

## Repository Structure

```text
rpi-edge-node/
├── ansible/
│   ├── ansible.cfg                    # Ansible settings and role search paths
│   ├── inventory.ini                  # Target gateway inventory
│   ├── site.yml                       # Main deployment playbook
│   ├── group_vars/
│   │   └── all/
│   │       ├── vars.yml               # Public infrastructure variables and ports
│   │       ├── vault.example.yml      # Reference template for secrets
│   │       └── vault.yml              # Encrypted production credentials
│   └── roles/
│       ├── base/                      # Kernel sysctl, UFW firewall, DNS hijack, CIFS mount
│       ├── singbox/                   # sing-box service, routing rules, DNS configuration
│       │   ├── templates/
│       │   │   ├── config.json.j2     # Jinja2 template for sing-box config
│       │   │   ├── sing-box.service.j2# systemd unit template
│       │   │   └── update-sing-box-lx.sh.j2 # Automated updater script
│       │   └── vars/
│       │       ├── vault_proton.example.yml # Reference template for Proton
│       │       └── vault_proton.yml   # Encrypted Proton VPN endpoints
│       ├── amneziawg/                 # In-kernel AmneziaWG setup and keys
│       ├── docker/                    # Docker container runtime
│       └── minecraft/                 # Minecraft server container & Restic backups
└── tools/
    └── proton-parser/                 # Go utility for Proton WireGuard config parsing
        ├── configs/                   # Inbox directory for raw .conf files (in .gitignore)
        ├── go.mod
        ├── main.go                    # Parser and vault encryptor source
        └── main_test.go               # Comprehensive unit test suite
```

---

## License

This project is released under the [MIT License](LICENSE). You are free to use, modify, and adapt it for your own network needs.
