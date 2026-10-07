[English](README.md) | **Русский**

# rpi-edge-node

> Автоматизированный домашний Edge-шлюз на Raspberry Pi с sing-box и Ansible

[![Ansible](https://img.shields.io/badge/Ansible-2.15+-EE0000?style=flat-square&logo=ansible&logoColor=white)](https://www.ansible.com/)
[![sing-box](https://img.shields.io/badge/sing--box-1.9+-000000?style=flat-square)](https://sing-box.sagernet.org/)
[![Platform](https://img.shields.io/badge/DietPi-Debian%20Bookworm-blue?style=flat-square&logo=debian)](https://dietpi.com/)
[![Architecture](https://img.shields.io/badge/Arch-ARM64-brightgreen?style=flat-square&logo=arm)](https://www.raspberrypi.com/)
[![Security](https://img.shields.io/badge/Security-Ansible%20Vault-critical?style=flat-square)](https://docs.ansible.com/ansible/latest/vault_guide/index.html)

## Обзор

**rpi-edge-node** автоматизирует развертывание и конфигурацию домашнего L3-шлюза на Raspberry Pi под управлением DietPi (Debian Bookworm ARM64). Проект настраивает sing-box в режиме TUN, VPN-сервер AmneziaWG для доступа удаленных клиентов, межсетевой экран UFW с поддержкой dual-stack и перехват DNS. Это обеспечивает прозрачную маршрутизацию, обход блокировок и интеграцию локальных сервисов для всех устройств сети без установки клиентских прокси-программ.

## Ключевые возможности

- **Прозрачное проксирование через sing-box TUN:** перехват транзитного сетевого трафика через L3 TUN-интерфейс с прямым пропуском (bypass) адресов RFC 1918 и локальных сетей (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `fd00::/8`, `fe80::/10`) для прямого доступа к локальным сервисам и обратным прокси (reverse proxy).
- **Полный перехват DNS (IPv4/IPv6):** правила перенаправления `PREROUTING` в UFW перехватывают транзитные UDP/TCP-запросы на 53 порт и отправляют их во встроенный резолвер sing-box, предотвращая утечки DNS и игнорируя жестко заданные DNS на клиентах.
- **Надежная dual-stack маршрутизация:** системные параметры sysctl настроены в режим `rp_filter = 2` (Loose mode), что предотвращает отбрасывание асимметричных пакетов между интерфейсами (`eth0`, `tun0`, `awg0`), а также используется TCP MSS clamping для предотвращения проблем с PMTU.
- **Интеграция AmneziaWG / WireGuard:** высокопроизводительный VPN-сервер на уровне ядра с поддержкой параметров обфускации для безопасного подключения удаленных клиентов к домашней сети.
- **Безопасное и идемпотентное монтирование SMB/CIFS:** подключение сетевого хранилища через `systemd-automount` с защитой точки монтирования неизменяемым атрибутом (`chattr +i`), предотвращающим заполнение корневого накопителя при недоступности NAS.

## Архитектура сети

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

## Структура проекта

```text
rpi-edge-node/
├── ansible/
│   ├── ansible.cfg                    # Параметры Ansible и пути поиска ролей
│   ├── inventory.ini                  # Инвентарь целевого узла
│   ├── site.yml                       # Основной playbook развертывания
│   ├── group_vars/
│   │   └── all/
│   │       ├── vars.yml               # Общедоступные параметры инфраструктуры и сети
│   │       ├── vault.example.yml      # Пример шаблона секретов
│   │       └── vault.yml              # Зашифрованные через Ansible Vault учетные данные
│   └── roles/
│       ├── base/                      # Системные пакеты, sysctl, фаервол UFW и перехват DNS
│       ├── singbox/                   # Прозрачный прокси, правила маршрутизации и DNS-резолвер
│       │   ├── templates/
│       │   │   └── config.json.j2     # Шаблон конфигурации sing-box
│       │   └── vars/
│       │       ├── vault_proton.example.yml  # Пример конфигурации узлов Proton WireGuard
│       │       └── vault_proton.yml          # Зашифрованные эндпоинты Proton VPN
│       ├── amneziawg/                 # Серверный эндпоинт AmneziaWG VPN
│       ├── docker/                    # Контейнерная среда Docker
│       └── minecraft/                 # Контейнеризированный сервер Minecraft
└── tools/
    └── proton-parser/                 # Go-утилита предобработки конфигов Proton WireGuard
        ├── configs/                   # Игнорируемая Git входящая папка для .conf файлов
        │   ├── <account1>/
        │   │   ├── <node-1>.conf
        │   │   └── <node-2>.conf
        │   └── <account2>/
        │       └── <node-3>.conf
        ├── go.mod
        ├── main.go
        └── main_test.go
```

## Быстрый старт

### Предварительные требования

- **Управляющая машина:** Ansible `>= 2.15` и Python 3.
- **Целевой узел:** Raspberry Pi 4B (или совместимый ARM64 одноплатник) с DietPi (Debian 12 Bookworm).
- **Доступ:** настроенный SSH-доступ по ключам с правами `sudo` без ввода пароля.

### Настройка конфигурации

1. **Клонируйте репозиторий и перейдите в каталог Ansible:**
   ```bash
   git clone https://codeberg.org/Retr0defeated/rpi-edge-node.git
   cd rpi-edge-node/ansible
   ```

2. **Настройте переменные с секретами:**
   ```bash
   cp group_vars/all/vault.example.yml group_vars/all/vault.yml
   ansible-vault encrypt group_vars/all/vault.yml
   ```

3. **Настройте узлы Proton VPN (Опционально):**
   Сгенерируйте `vault_proton.yml` автоматически через [конвейер Proton WireGuard](#конвейер-proton-wireguard-toolsproton-parser) или скопируйте файл-пример:
   ```bash
   cp roles/singbox/vars/vault_proton.example.yml roles/singbox/vars/vault_proton.yml
   ansible-vault encrypt roles/singbox/vars/vault_proton.yml
   ```

4. **Сконфигурируйте инвентарь и параметры сети:**
   Проверьте и настройте `inventory.ini` (или `inventory/hosts.yml`) и `group_vars/all/vars.yml` под ваши подсети, IP-адрес шлюза и порты сервисов.

### Развертывание

Запустите playbook на целевом узле:

```bash
ansible-playbook -i inventory.ini site.yml --ask-vault-pass
```

> Если используется файл инвентаря вроде `inventory/hosts.yml`, укажите флаг `-i inventory/hosts.yml`.

## Конвейер Proton WireGuard (`tools/proton-parser`)

CLI-утилита `tools/proton-parser` автоматизирует извлечение и преобразование скачанных `.conf` файлов Proton VPN WireGuard в переменные Ansible для роли `singbox`.

### Структура входящих файлов

Разместите скачанные `.conf` файлы WireGuard по папкам соответствующих аккаунтов в `tools/proton-parser/configs/` (папка игнорируется Git):

```text
tools/proton-parser/configs/
├── acc1/
│   ├── nl-101.conf
│   └── nl-102.conf
└── acc2/
    └── nl-201.conf
```

### Использование

Запустите утилиту из каталога инструмента или из корня репозитория:

```bash
cd tools/proton-parser

# Парсинг конфигов и генерация незашифрованного vault_proton.yml
go run main.go

# Парсинг конфигов с автоматическим шифрованием через Ansible Vault (автообнаружение .vault_pass)
go run main.go --encrypt

# Указание явного файла пароля или коротких флагов при необходимости
go run main.go -e -p /path/to/.vault_pass
```

Флаги CLI:
* `-i, --in`: Путь к исходной папке с конфигурациями (по умолчанию: `./tools/proton-parser/configs` или `./configs`).
* `-o, --out`: Путь назначения выходного YAML (по умолчанию: `./ansible/roles/singbox/vars/vault_proton.yml`).
* `-e, --encrypt`: Шифрование файла на месте через `ansible-vault encrypt` (автоматически находит `.vault_pass` по дереву каталогов, если не передан явно).
* `-p, --vault-pass-file, --vault-password-file`: Необязательный путь к файлу пароля Ansible Vault.

### Схема конфигурации (`vault_proton.yml`)

Сгенерированный файл соответствует следующей структуре:

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

## Безопасность и секреты

- **Шифрование секретов:** все чувствительные параметры (приватные ключи VPN, параметры обфускации, пароли к хранилищу и внутренние DNS-записи) зашифрованы через **Ansible Vault**.
- **Защита от утечек в логи:** шаблоны конфигураций развертываются с правами `mode: '0600'` и директивой `diff: false`, исключающей попадание паролей в стандартный вывод Ansible.
- **Защита файловой системы:** точка монтирования сетевого хранилища защищена атрибутом `chattr +i` в размонтированном состоянии, что гарантирует целостность локального накопителя при сбоях в сети или перезагрузке NAS.

