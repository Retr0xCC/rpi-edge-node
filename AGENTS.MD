# Antigravity Rules & Engineering Guidelines: Raspberry Pi Edge Gateway

> [!IMPORTANT]
> **Agent Communication Language / Язык общения агента:**
> - **User Dialogue & Interaction:** Always conduct dialogue, reply to user prompts, explain architectural decisions, and provide instructions in **Russian** (`ru-RU`).
> - **Code & Technical Artifacts:** All code, variables, configuration keys, file contents, YAML comments, documentation, and Git commit messages must strictly remain in **English**.

> **Target Node:** Raspberry Pi 4B (DietPi OS / Debian GNU/Linux Bookworm, ARM64)  
> **Repository:** [`rpi-edge-node`](.)  
> **Primary Role:** Edge Gateway, Smart Transparent Proxy ([`sing-box`](ansible/roles/singbox)), WireGuard/AmneziaWG Endpoint ([`amneziawg`](ansible/roles/amneziawg)), Local DNS Resolver (Port 53), Stateful Firewall ([UFW](ansible/roles/base)), and Containerized Services ([`docker`](ansible/roles/docker), [`minecraft`](ansible/roles/minecraft)).

---

## 1. Project Overview & Network Context

### 1.1. Role of the Node
The node is a dedicated Raspberry Pi 4B running **DietPi** (minimal Debian Bookworm ARM64), configured as an authoritative edge gateway for the local network via Ansible ([`site.yml`](ansible/site.yml)). Core responsibilities include:
- **Transparent traffic routing and proxying** via sing-box (`tun0` interface).
- **VPN ingress/egress gateway** via kernel-level AmneziaWG (`awg0` interface).
- **Centralized DNS routing** on port 53 (local DNS resolution and split-DNS for internal domains).
- **Stateful firewalling and NAT masquerading** via UFW and iptables.
- **Edge application hosting** (Minecraft server with automated SMB/CIFS backup mounts to NAS).

### 1.2. Network Boundaries & Topology
- **Local Gateway Subnet (`eth0`):** `192.168.140.0/24`.
- **Physical Hardware Gateway:** `192.168.140.254` (upstream ISP router providing DHCP leases and default IPv4 route).
- **Reverse Proxy / Nginx Proxy Manager (NPM):** `192.168.160.250` (located in adjacent local subnet `192.168.160.0/24`). All traffic to NPM and local subnets **must bypass proxy tunnels directly via LAN routing**.
- **Private RFC1918 Subnets:** `192.168.0.0/16`, `10.0.0.0/8`, `172.16.0.0/12`.
- **AmneziaWG VPN Subnet (`awg0`):** `10.9.9.0/24` (Gateway: `10.9.9.1/24`, IPv6: `fd00:9:9::1/64`, Listen Port: `19302/udp`).
- **sing-box TUN Network (`tun0`):** `172.19.0.1/30` (IPv4) and `fdfe:dcba:9876::1/126` (IPv6).
- **Static IPv6 on Gateway:** `fd00::1/64` on `eth0`.

### 1.3. Architecture & Interface Guardrails
- **`eth0` is dynamically managed via DHCP:** Do **NOT** rewrite `/etc/network/interfaces` with manual static IPv4 routes, custom gateway directives, or destructive template overwrites. Overwriting this file will kill DHCP client negotiation and isolate the Raspberry Pi from the network permanently.
- **Static IPv6 block:** Static IPv6 on `eth0` is injected strictly via `ansible.builtin.blockinfile` in [`ansible/roles/base/tasks/main.yml`](ansible/roles/base/tasks/main.yml) without altering dynamic IPv4 interface configuration.
- **DietPi Native Integrations:** Respect native DietPi utilities:
  - ZRAM Swap: `/boot/dietpi/func/dietpi-set_swapfile zram`.
  - Docker & Compose Engine: `/boot/dietpi/dietpi-software install 134`.
  - Dynamic NTP Mirror: `CONFIG_NTP_MIRROR=Gateway` in `/boot/dietpi.txt`.

---

## 2. Security & Ansible Vault Protocol

### 2.1. Strict Prohibitions
1. **STRICTLY PROHIBITED:** Opening, reading, searching via tools, or modifying master password files: `.vault_pass`, `.vault_pass*`, `*.vault_pass`, `.vault-password*`, `*.vault_password` across all directories.
2. **STRICTLY PROHIBITED:** Attempting to decrypt or edit encrypted vault files directly ([`ansible/group_vars/all/vault.yml`](ansible/group_vars/all/vault.yml) or `*.vault.yml`).
3. **STRICTLY PROHIBITED:** Running `ansible-vault` commands in the terminal with `--vault-password-file`.
4. **STRICTLY PROHIBITED:** Exposing actual secrets, passwords, private keys, tokens, or obfuscation parameters in plaintext in playbooks, variables, templates, or chat responses.

### 2.2. Protocol for New Vault Variables
Whenever a new encrypted credential or secret variable is required:
1. **Never attempt to encrypt the value yourself** or generate fake encrypted blocks in code.
2. Declare a variable reference in unencrypted configuration files ([`ansible/group_vars/all/vars.yml`](ansible/group_vars/all/vars.yml) or `defaults/main.yml`) using the standard `vault_` prefix:
   ```yaml
   singbox_secret_key: "{{ vault_singbox_secret_key }}"
   ```
3. **Provide the user with an instruction block in chat (in Russian)** containing:
   - The exact required variable name (e.g., `vault_singbox_secret_key`).
   - The intended purpose or placeholder data structure.
   - A ready-to-run copy-paste terminal command for manual local encryption:
     ```bash
     ansible-vault encrypt_string 'YourSecretPassword' --name 'vault_singbox_secret_key'
     ```
   - An instruction asking the user to append the encrypted output to [`ansible/group_vars/all/vault.yml`](ansible/group_vars/all/vault.yml).

### 2.3. Task Hardening for Sensitive Files
When deploying configuration files containing sensitive credentials (e.g., [`config.json.j2`](ansible/roles/singbox/templates/config.json.j2), [`awg0.conf.j2`](ansible/roles/amneziawg/templates/awg0.conf.j2), `.env.j2`):
- Enforce strict file permissions: `mode: '0600'` (or `'0640'`).
- Always set `diff: false` in `ansible.builtin.template` tasks to prevent decrypted secrets from leaking into Ansible execution logs.

### 2.4. Proton WireGuard Pipeline & Vault Boundary
- Raw `.conf` configurations downloaded from Proton VPN reside strictly in `tools/proton-parser/configs/` (must remain in `.gitignore`).
- The generated output [`ansible/roles/singbox/vars/vault_proton.yml`](ansible/roles/singbox/vars/vault_proton.yml) contains live WireGuard private keys and **must always be encrypted via Ansible Vault** (generated with `--encrypt`).
- The Ansible `singbox` role executes `tools/proton-parser` automatically with `--encrypt`. The Go tool auto-discovers `.vault_pass` across repository parent paths.
- Reference templates ([`vault_proton.example.yml`](ansible/roles/singbox/vars/vault_proton.example.yml)) must only contain sanitized placeholders (`PRIVATE_KEY_HERE`, `PUBLIC_KEY_HERE`). Plaintext keys must never be committed.

---

## 3. Ansible Coding Standards & Linting

### 3.1. Fully Qualified Collection Names (FQCN)
All Ansible modules and action plugins **must** use FQCN without exception. Short names (`template`, `copy`, `apt`) are prohibited:
- `ansible.builtin.apt`
- `ansible.builtin.template`
- `ansible.builtin.copy`
- `ansible.builtin.file`
- `ansible.builtin.systemd`
- `ansible.builtin.command` / `ansible.builtin.shell`
- `ansible.builtin.lineinfile` / `ansible.builtin.blockinfile`
- `ansible.builtin.user`
- `community.general.ufw`
- `community.general.timezone`

### 3.2. Strict Boolean Types
Use only lowercase YAML boolean literals (`true` / `false`):
```yaml
# Correct:
update_cache: true
become: true
enabled: false
state: present

# Prohibited in tasks:
update_cache: yes
become: True
enabled: "no"
```
*(Exception: String values required by external daemon configs that explicitly expect strings, e.g., `ssh_permit_root_login: "no"`).*

### 3.3. ansible-lint Production Profile Compliance
The codebase strictly complies with the **production** profile of `ansible-lint` (`ansible-lint ansible/site.yml`):
- **Task Naming:** Every task must have a clear, unique, and descriptive `name:`.
- **Command Idempotency:** Any task using `ansible.builtin.command` or `ansible.builtin.shell` must declare `changed_when:` and `failed_when:` (or `args: creates:`).
- **Handlers:** Service restarts and configuration reloads (`Reload sysctl`, `Reload ufw`, `Restart sing-box service`) must be triggered via `notify:`, rather than invoking service management modules directly within regular tasks.

### 3.4. YAML Formatting & Visual Structure
- Always begin YAML files with the document start marker `---`.
- Preserve the project's visual section dividers:
  ```yaml
  # --------------------------------------------------------------------
  # 1. Base Packages Installation
  # --------------------------------------------------------------------
  ```
- Use 2-space indentation. Maintain spaces inside Jinja2 template delimiters: `{{ item }}`, `{{ vault_var }}`.

---

## 4. Sing-box & Networking Guardrails

### 4.1. LAN Isolation in `route_exclude_address` (TUN Inbound)
In [`ansible/roles/singbox/templates/config.json.j2`](ansible/roles/singbox/templates/config.json.j2), the `tun-in` inbound interface operates with `auto_route: true`, `auto_redirect: true`, and `strict_route: true`.

> [!CAUTION]
> The `route_exclude_address` array **MUST ALWAYS** contain all private and local subnets:
> ```json
> "route_exclude_address": [
>   "10.0.0.0/8",
>   "172.16.0.0/12",
>   "192.168.0.0/16",
>   "fd00::/8",
>   "fe80::/10"
> ]
> ```
> **Why this is critical:** Removing or altering these exclusions causes the TUN inbound adapter to intercept all local LAN traffic. Requests to the physical gateway (`192.168.140.254`) and Nginx Proxy Manager (`192.168.160.250`) will be forwarded into the proxy tunnel rather than passing directly through `eth0`. This triggers **HTTP 502 Bad Gateway** errors on reverse proxies and causes complete loss of access to local network services.

### 4.2. Asymmetric Routing & `rp_filter = 2` (Loose Mode)
Because the Raspberry Pi operates as a multi-homed gateway (`eth0`, `tun0`, `awg0`), traffic flows along asymmetric routing paths.

> [!IMPORTANT]
> The kernel reverse path filtering parameter `rp_filter` **MUST** remain set to **`2` (Loose mode)** across all configurations:
> 1. In system sysctl [`ansible/roles/base/templates/99-edge-gateway.conf.j2`](ansible/roles/base/templates/99-edge-gateway.conf.j2):
>    ```ini
>    net.ipv4.conf.all.rp_filter = 2
>    net.ipv4.conf.default.rp_filter = 2
>    net.ipv4.conf.eth0.rp_filter = 2
>    ```
> 2. In `/etc/ufw/sysctl.conf` via task in [`ansible/roles/base/tasks/main.yml`](ansible/roles/base/tasks/main.yml):
>    ```ini
>    net/ipv4/conf/all/rp_filter=2
>    net/ipv4/conf/default/rp_filter=2
>    ```
> **Never revert `rp_filter` to `1` (Strict mode).** In strict mode, the Linux kernel silently drops incoming packets from the LAN and VPN tunnels whenever the reverse routing path uses a different interface.

### 4.3. Direct DNS Listener on Port 53
- sing-box functions as the primary DNS resolver for the gateway and local clients via inbound `dns-in`:
  ```json
  {
    "type": "direct",
    "tag": "dns-in",
    "listen": "::",
    "listen_port": 53
  }
  ```
- The `systemd-resolved` service **must remain permanently stopped, disabled, and masked** (`masked: true`) to avoid port 53 binding conflicts.
- `/etc/resolv.conf` is maintained as a static file pointing to the primary upstream DNS resolver (`vault_primary_dns_server`).
- Internal domain zones (`example.home`, `.home`, `.lan`, `.local` or `{{ vault_base_domain }}`) in sing-box must always route queries to the `local` DNS server (`vault_singbox_dns_local`).

### 4.4. UFW, Packet Forwarding, and TCP MSS Clamping
- The forwarding policy in `/etc/default/ufw` must be set to `DEFAULT_FORWARD_POLICY="ACCEPT"`, and UFW rules must specify `direction: routed, policy: allow`.
- NAT masquerading rules in `/etc/ufw/before.rules` must be preserved for local LAN (`192.168.0.0/16`) and VPN (`10.9.9.0/24`) traffic routed through `eth0`.
- To avoid PMTU black holes and packet fragmentation across WireGuard and sing-box tunnels, TCP MSS clamping rules must remain active in `before.rules` and `before6.rules`:
  ```text
  -A ufw-before-forward -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu
  -A ufw6-before-forward -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu
  ```

### 4.5. Proton VPN Outbound Topology & Failover
- **Hierarchical Outbounds:** When Proton accounts are loaded via [`vault_proton.yml`](ansible/roles/singbox/vars/vault_proton.yml):
  1. Each WireGuard endpoint is declared in `endpoints` with `detour: "proxy"` (routed via main proxy to bypass ISP SNI/protocol blocking).
  2. Per-account failover groups (`urltest`) maintain 1 active node per Proton account (`proton-group-<account>`) with anti-flapping parameters (`url: "https://cp.cloudflare.com/generate_204"`, `interval: "10m"`, `tolerance: 50`, `idle_timeout: "30m"`).
  3. Specific routes target dedicated account groups directly (e.g. Google traffic routes to the first Proton account group). The legacy top-level round-robin balancer `proton-balance-nl` is omitted.
- **Graceful Fallback:** If `singbox_proton_accounts` is empty or missing, targeted rules fallback to `proxy` directly. Legacy `smart` DoT DNS is deprecated and removed.

---

## 5. Git & Commit Conventions

All commits must strictly adhere to the **Conventional Commits** specification in lowercase with an explicit `scope`:

```text
<type>(<scope>): <short imperative description>
```

### 5.1. Allowed Types (`type`)
- `feat`: New feature, role, or service deployment.
- `fix`: Bug fix, routing correction, firewall rule repair.
- `refactor`: Code reorganization without functional behavior changes.
- `style`: Formatting, linting fixes, FQCN migration, decorative comment banners.
- `security`: Vault updates, secret rotation, ACL hardening.
- `ci`: GitHub Actions workflows and build script changes.
- `docs`: Documentation updates and agent guidelines.

### 5.2. Standard Scopes (`scope`)
- `gateway` — cross-cutting network routing, sysctl parameters, dual-stack changes.
- `singbox` — proxy configuration, TUN inbound, DNS rules, outbounds.
- `tools` — auxiliary CLI utilities and pre-processors (`tools/proton-parser`).
- `amneziawg` — kernel interface, obfuscation settings, VPN peers.
- `base` — system packages, sshd, zram swap, CIFS mounts, shell.
- `ufw` — firewall rules, NAT masquerading, `before.rules`.
- `ansible` — playbooks, inventory, linting configurations.
- `minecraft` — game server configuration, environment files, backups.
- `docker` — Docker daemon setup and container engine tasks.

### 5.3. Reference Commits from Project History
```text
refactor(singbox): remove redundant source ip routing rule
fix(gateway): resolve npm 502 error and exclude local subnets from tun capture
feat(gateway): enable dual-stack ipv6 routing, ufw6 rules, and lan nat bypass
fix(ansible): resolve cifs fstab option escaping and sync outbound ssh keypair
style(ansible): enforce FQCN, strict booleans, and ansible-lint best practices
fix(ufw): allow awg0 and tun0 traffic in before.rules to bypass docker forward drop
security: relocate internal dns addresses to vault to prevent topology leak
```
