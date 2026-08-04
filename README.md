# ssh-pve

A terminal UI for listing Proxmox VE VMs across a whole cluster and shelling
into them with a single keypress.

`ssh-pve` connects to a Proxmox VE cluster via the REST API, inventories every
QEMU virtual machine (name, ID, node, run status), enriches each VM with the
IPv4/IPv6 addresses its QEMU guest agent reports, and presents them in a
scrollable list. Pressing Enter on a VM exits the TUI and hands the terminal
over to `ssh`.

## Requirements

- Proxmox VE 8.x or 9.x.
- An API token (see [Permissions](#permissions))
- The QEMU guest agent installed and running inside each VM you want to reach.
- Go 1.26 or newer (only if building from source).

## Install

Homebrew (macOS/Linux):

```sh
brew install abinnovision/tap/ssh-pve
```

With the Go toolchain:

```sh
go install github.com/abinnovision/ssh-pve@latest
```

Build from a checkout:

```sh
git clone https://github.com/abinnovision/ssh-pve
cd ssh-pve
make build # Builds to ./dist/ssh-pve
make install # Moves binary to ~/.local/bin/ssh-pve
```

Or download a prebuilt binary (darwin/linux, amd64/arm64) from the
[releases page](https://github.com/abinnovision/ssh-pve/releases).

## Quick start

1. Create an API token with the right permissions - see
   [Permissions](#permissions).
2. Run `ssh-pve`. On first launch the onboarding form appears:

   ```
   ssh-pve - cluster onboarding

   ▸ Cluster Endpoints (required)
     Comma-separated PVE API URLs (including /api2/json), tried in order...
     https://pve1.example.com:8006/api2/json___________________________

     API Token ID (required)
     ...
   ```

3. Fill in the fields, press Enter. The TUI verifies the connection by
   listing VMs; on failure it returns to the form with an error.
4. On success the VM list appears. Hover or select a row to reveal its IPs
   (IPv4 preferred - IPv6 shown only when no IPv4 exists). Press Enter to SSH.

## Configuration

The config file lives at `~/.config/ssh-pve/config.yaml` (or
`$XDG_CONFIG_HOME/ssh-pve/config.yaml` when set). The file is mode 0600 because
it holds the API token secret.

```yaml
endpoints:
  - https://pve1.example.com:8006/api2/json
  - https://pve2.example.com:8006/api2/json
api_token_id: inventory@pve!readonly
api_token_secret: 11111111-2222-3333-4444-555555555555
default_ssh_user: root
ssh_command_template: ssh {{user}}@{{ip}}        # optional
insecure: false                                  # optional, default false
vm_overrides:                                    # optional
  100:
    ssh_user: debian
    ssh_command_template: ssh -i ~/.ssh/debian {{user}}@{{ip}}
```

| Field | Description |
| --- | --- |
| `endpoints` | Ordered list of PVE API URLs including `/api2/json`. Tried in turn so a down node falls through. At least one required. |
| `api_token_id` | Full token identifier, `user@realm!tokenname`. |
| `api_token_secret` | The token secret UUID shown once at creation. |
| `default_ssh_user` | SSH user used when no per-VM override applies. |
| `ssh_command_template` | Command template with `{{user}}` and `{{ip}}` placeholders. Empty falls back to `ssh {{user}}@{{ip}}`. |
| `insecure` | Skip TLS certificate verification. For lab clusters with self-signed certs. Default `false`. |
| `vm_overrides` | Map of VMID → override. Each field is optional; empty means inherit the top-level value. |

### SSH command template

Templates use simple `{{user}}` / `{{ip}}` substitution - no shell escaping is
applied, so keep the template under your control. Examples:

```yaml
ssh_command_template: ssh -i ~/.ssh/id_ed25519 {{user}}@{{ip}}
ssh_command_template: ssh -J jump.example.com {{user}}@{{ip}}
```

### Keybindings

| Key | Action |
| --- | --- |
| `↑` / `k` | Move selection up |
| `↓` / `j` | Move selection down |
| `Enter` | SSH to selected VM (TUI exits, ssh takes over the terminal) |
| `q` / `Esc` | Quit |
| `Tab` / `Shift+Tab` | Cycle form fields (onboarding) |
| `Space` | Toggle checkbox (onboarding) |

## Permissions

The token needs these privileges:

| API endpoint | Privilege | Path |
| --- | --- | --- |
| `/cluster/resources` | `Sys.Audit` | `/` |
| `/nodes/{node}/qemu/{vmid}/status/current`, `/config` | `VM.Audit` | `/vms/{vmid}` |
| `/nodes/{node}/qemu/{vmid}/agent/network-get-interfaces` | `VM.GuestAgent.Audit` (PVE 9.x) / `VM.Monitor` (PVE 8.x) | `/vms/{vmid}` |

### CLI setup

With privilege separation on, the token's effective permissions are the
**intersection** of the user's and the token's ACLs, so both must carry every
role - a token-only ACL grants nothing.

```sh
# PVE 9.x - PVEAuditor covers all three privileges
pveum user add inventory@pve --password "$(openssl rand -base64 24)"
pveum user token add inventory@pve readonly -privsep 1
pveum acl modify / -user  'inventory@pve'          -role PVEAuditor
pveum acl modify / -token 'inventory@pve!readonly' -role PVEAuditor
```

```sh
# PVE 8.x - PVEAuditor lacks VM.Monitor, so add a custom role on /vms
pveum user add inventory@pve --password "$(openssl rand -base64 24)"
pveum user token add inventory@pve readonly -privsep 1
pveum role add GuestAgentAudit -privs "VM.Monitor"
pveum acl modify /     -user  'inventory@pve'          -role PVEAuditor
pveum acl modify /     -token 'inventory@pve!readonly' -role PVEAuditor
pveum acl modify /vms  -user  'inventory@pve'          -role GuestAgentAudit
pveum acl modify /vms  -token 'inventory@pve!readonly' -role GuestAgentAudit
```
