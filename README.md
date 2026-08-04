# ssh-pve

A terminal UI for listing Proxmox VE VMs across a whole cluster and shelling
into them with a single keypress.

`ssh-pve` connects to a Proxmox VE cluster via the REST API, inventories every
QEMU virtual machine (name, ID, node, run status), enriches each VM with the
IPv4/IPv6 addresses its QEMU guest agent reports, and presents them in a
scrollable list. Press Enter on a VM and the TUI exits, handing the terminal
to `ssh` so the session looks like you ran it yourself.

- **Cluster-wide** — lists VMs from every node in one view, no per-node login.
- **Guest-agent IPs** — addresses come from inside the guest, not the PVE
  management network, so you reach the VM's real interface.
- **Endpoint fallback** — configure multiple API endpoints; the first
  reachable one wins.
- **Per-VM overrides** — different SSH user or command template per VMID.
- **Onboarding** — first launch with no config file walks you through setup
  and verifies the connection before saving.
- **Keyboard + mouse** — full alt-buffer TUI with hover, click, scroll wheel.

## Requirements

- Proxmox VE 8.x or 9.x (9.x recommended — `PVEAuditor` alone covers all
  required permissions; see [Permissions](#permissions) below).
- The QEMU guest agent installed and running inside each VM you want to reach.
- Go 1.26 or newer (only if building from source).

## Install

```sh
go install github.com/abinnovision/ssh-pve@latest
```

Or build from a checkout:

```sh
git clone https://github.com/abinnovision/ssh-pve
cd ssh-pve
go build -o ssh-pve .
```

## Quick start

1. Create an API token with the right permissions — see
   [Permissions](#permissions).
2. Run `ssh-pve`. On first launch the onboarding form appears:

   ```
   ssh-pve — cluster onboarding

   ▸ Cluster Endpoints (required)
     Comma-separated PVE API URLs (including /api2/json), tried in order...
     https://pve1.example.com:8006/api2/json___________________________

     API Token ID (required)
     ...
   ```

3. Fill in the fields, press Enter. The TUI verifies the connection by
   listing VMs; on failure it returns to the form with an error.
4. On success the VM list appears. Hover or select a row to reveal its IPs
   (IPv4 preferred — IPv6 shown only when no IPv4 exists). Press Enter to SSH.

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
ssh_command_template: ssh {{user}}@{{ip}        # optional
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

Templates use simple `{{user}}` / `{{ip}}` substitution — no shell escaping is
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
| Mouse hover | Reveal IPs for the row under the cursor |
| Mouse click | Select the clicked row |
| Scroll wheel | Scroll the list |

## Permissions

`ssh-pve` needs a read-only API token. The token authenticates via the
`Authorization` header; the backing user's password is never checked, so a
service account with no password (unable to log in to the web UI) is the
recommended setup.

The token needs three privileges on three paths:

| API endpoint | Privilege | Path |
| --- | --- | --- |
| `/cluster/resources` | `Sys.Audit` | `/` |
| `/nodes/{node}/qemu/{vmid}/status/current`, `/config` | `VM.Audit` | `/vms/{vmid}` |
| `/nodes/{node}/qemu/{vmid}/agent/network-get-interfaces` | `VM.GuestAgent.Audit` | `/vms/{vmid}` |

**PVE 9.x** — the built-in `PVEAuditor` role grants all three (it gained
`VM.GuestAgent.Audit` when `VM.Monitor` was split into the `VM.GuestAgent.*`
family). Assign `PVEAuditor` to the **token** (not the user) with privilege
separation on, and you're done.

**PVE 8.x** — `PVEAuditor` lacks `VM.GuestAgent.Audit`. Create a custom role
with that privilege and assign it alongside `PVEAuditor`.

### GUI setup (PVE 9.x)

1. **Create the service user**
   `Datacenter` → `Permissions` → `Users` → `Add`
   - User name: `inventory`
   - Realm: `PVE`
   - No password needed — the token authenticates independently.

2. **Create the API token** (copy the secret now — shown once)
   `Datacenter` → `Permissions` → `API Tokens` → `Add`
   - User: `inventory@pve`
   - Token ID: `readonly`
   - **Privilege Separation: enabled** (the default; critical — without it
     the token inherits the user's full permissions)
   - Click `Create`, copy the displayed secret.

3. **Grant the token `PVEAuditor` on `/`**
   `Datacenter` → `Permissions` → `Add` → `API Permission`
   - Path: `/`
   - User/Token: `inventory@pve!readonly` (the token, not the user)
   - Role: `PVEAuditor`
   - Propagate: checked

> **Gotcha**: with privilege separation on, the token's effective permissions
> are the intersection of the user's and the token's ACLs. If you assign
> `PVEAuditor` to the **user** `inventory@pve` but not the **token**
> `inventory@pve!readonly`, the token gets nothing and the VM list comes back
> empty. Assign the role to the token.

### CLI equivalent

```sh
pveum user add inventory@pve
pveum user token add inventory@pve readonly -privsep 1
pveum acl modify / -token 'inventory@pve!readonly' -role PVEAuditor
```

## Project layout

```
.
├── main.go          # entry point — calls tui.Run()
├── pve/             # cluster inventory client (go-proxmox wrapper)
│   ├── client.go    #   New, ListVMs, bounded-concurrency guest-agent fetch
│   └── vm.go        #   VM type, PrimaryIPv4/PrimaryIPv6, Running
├── config/          # on-disk YAML config store
│   ├── config.go    #   Config, VMOverride, Validate, ResolveSSH, Default
│   └── io.go        #   Load, Save, ConfigPath, Exists (XDG-aware)
└── tui/             # bubbletea v2 terminal UI
    ├── tui.go       #   model, state machine, Run, SSH hand-off
    ├── onboarding.go#   config form with connection test
    ├── vmlist.go    #   scrollable VM list with hover-reveal IPs
    └── styles.go    #   shared lipgloss palette
```

## License

MIT
