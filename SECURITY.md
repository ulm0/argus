# Security Policy

## Supported Versions

Only the latest release receives security fixes. Older releases are not maintained.

| Version | Supported |
|---------|-----------|
| Latest  | Yes       |
| Older   | No        |

## Reporting a Vulnerability

**Do not open a public issue for security vulnerabilities.**

Please report security issues privately via one of the following channels:

- **GitHub Security Advisories:** [Report a vulnerability](https://github.com/ulm0/argus/security/advisories/new)
- **Email:** If you cannot use GitHub, contact the maintainer directly through the profile linked on the repository.

Include as much detail as possible:

- A description of the vulnerability and its potential impact
- Steps to reproduce or a proof-of-concept
- Affected version(s) and target platform (e.g., Raspberry Pi OS Lite)
- Any suggested mitigations, if applicable

You can expect an acknowledgement within **72 hours** and a resolution or status update within **14 days**.

## Threat Model

Argus is designed to run on a private local network (home WiFi or USB gadget mode). The threat model reflects this:

- **Assumed trusted network:** The web interface binds to all interfaces by default. It should only be exposed on a trusted LAN or over a VPN — never directly to the internet.
- **Local attacker:** Physical access to the device is considered out of scope. Disk images and config files are stored unencrypted.
- **Credentials in config:** Telegram bot tokens, Samba passwords, and the web secret key are stored in `config.yaml` in plain text. This file should be readable only by the service user (`chmod 600`).
- **Telegram alerting:** Video clips are transmitted to the configured Telegram bot over HTTPS. No other external network connections are made unless auto-update is enabled.

## Security Hardening Recommendations

When deploying Argus in a production environment:

1. **Restrict config file permissions:**
   ```bash
   chmod 600 ~/.argus/config.yaml
   ```

2. **Change the default web secret key** before first use — `argus generate` auto-generates one, but verify it is set in `config.yaml`.

3. **Bind the web port to localhost** if accessing via a local reverse proxy or SSH tunnel, and keep the default port (80) firewalled from untrusted interfaces.

4. **Use a dedicated Telegram bot** with minimal permissions (send messages only, restricted to a private chat).

5. **Enable the offline AP** only when needed — the hotspot uses WPA2 but expands the network attack surface.

6. **Keep Argus up to date** — run `sudo argus upgrade` periodically or enable `update.auto_update: true` in `config.yaml`.

## Known Limitations

- The web interface has no built-in authentication layer. Access control must be enforced at the network level (firewall, VPN) or via a reverse proxy with authentication (e.g., Caddy, nginx with basic auth).
- Disk images (`.img` files) are not encrypted at rest.
- The offline AP passphrase is stored in plaintext in `config.yaml`.
