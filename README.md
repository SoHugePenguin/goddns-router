# GoDDNS-Router

A lightweight DDNS updater written in Go, designed for embedded Linux routers such as OpenWRT and iStoreOS-N100.

This tool automatically creates, updates, and removes DNS records via the Cloudflare API, based on the current active devices discovered through the IPv6 neighbor table (ip -6 neigh) and MAC address matching.

No daemon is required, and the binary is self-contained with zero runtime dependencies (CGO_ENABLED=0).

> 🧠 The tool is ideal for **router environments**. If run on a PC or VM, it can usually only update the DDNS for the **local machine itself**, due to limited access to neighbor entries.

---

## ✨ Features

- [x] ⚙️ Zero-dependency binary for Linux (built with `CGO_ENABLED=0`)
- [x] 🧩 Supports Cloudflare DNS API
- [x] 📡 Full IPv6 device discovery via `ip neigh` + MAC matching
- [x] 🔁 Suitable for periodic execution (e.g., via `crontab`)
- [x] 🧠 Simple JSON-based configuration
- [x] 🖥️ Also usable on personal machines (e.g., DMZ server setup)

---

## 🛠 Build (for Linux AMD64)

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o router-ddns ./
```

## 🚀 Usage

- Copy the binary and a config.json file to your router or Linux server.

- Add a scheduled task to run the binary periodically (e.g., via crontab).

- Each execution will scan your LAN and update DNS records via Cloudflare.

## 📄 config.json Example (multi-device)
```json
{
  "ownIpv4Enabled": false,
  "uniqueToken": "iStoreOS-N100",
  "cloudflareEmail": "your-cloudflare-email",
  "cloudflareApiKey": "your-cloudflare-api-key",
  "domainName": "your-cloudflare-domain-name",
  "localIpv4AddrApiUrl": "https://api-ipv4.ip.sb/ip",
  "localIpv6AddrApiUrl": "https://api-ipv6.ip.sb/ip",
  "recordMap": {
    "00:00:00:00:00:00": [
      {
        "name": "n100",
        "comment": "iStoreOS-N100 router",
        "proxy": false
      }
    ],
    "10:ff:e0:06:7d:f5": [
      {
        "name": "mz72",
        "comment": "PVE remote management",
        "proxy": false
      }
    ],
    "a0:36:9f:f7:f7:d5": [
      {
        "name": "pve",
        "comment": "Proxmox host",
        "proxy": false
      }
    ],
    "bc:24:11:40:47:48": [
      {
        "name": "virt210",
        "comment": "Windows Server 2022 - Penguin",
        "proxy": false
      }
    ],
    "bc:24:11:42:15:81": [
      {
        "name": "virt211",
        "comment": "Windows Server 2022 - XiaoYan",
        "proxy": false
      },
      {
        "name": "xiaoyan",
        "comment": "XiaoYan alias",
        "proxy": false
      }
    ]
  }
}
```


## 🖥️ Minimal Config (Local-only DDNS)

- If you're only using this on a single device (e.g., your personal computer):

```json
{
  "ownIpv4Enabled": true,
  "uniqueToken": "my-computer001",
  "cloudflareEmail": "your-cloudflare-email",
  "cloudflareApiKey": "your-cloudflare-api-key",
  "domainName": "your-cloudflare-domain-name",
  "localIpv4AddrApiUrl": "https://api-ipv4.ip.sb/ip",
  "localIpv6AddrApiUrl": "https://api-ipv6.ip.sb/ip",
  "recordMap": {
    "00:00:00:00:00:00": [
      {
        "name": "your-name",
        "comment": "My computer",
        "proxy": false
      },
      {
        "name": "your-name2",
        "comment": "Alias name",
        "proxy": false
      }
    ]
  }
}
```

## 🧠 Notes

- MAC addresses are matched to IPv6 addresses via `ip -6 neigh`, so this tool requires access to your LAN's neighbor cache.
- This tool does not run as a daemon; each execution updates once and exits.
- Make sure Cloudflare API credentials have permissions to manage DNS records.
- All records updated are of type `AAAA` (IPv6).

## 📄 License

Copyright © 2025 SoHugePenguin

This project is licensed under the GNU Lesser General Public License v3.0.
See the LICENSE file for full details.