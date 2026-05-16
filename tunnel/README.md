# tunnel/

Runtime config for **cloudflared** on the Mac mini gateway. This directory holds the *example* shape of the files that get rendered onto disk at `/usr/local/etc/cloudflared/` and `/Library/LaunchDaemons/`.

The cloud-plane *description* of the same tunnel — the source of truth for "what does this tunnel do" — is `../infra/tunnel.cloudflare.yaml`. This directory is the *operational* form cloudflared consumes.

## Files

| File | Installed at | Notes |
|------|--------------|-------|
| `config.example.yml` | `/usr/local/etc/cloudflared/config.yml` | Daemon config; references `<UUID>.json` for credentials |
| `ingress.yml` | (optional, inlined in config) | Ingress fragment, kept in sync with the inline copy in `config.example.yml` |
| `service.example.plist` | `/Library/LaunchDaemons/ai.owera.cloudflared.plist` | LaunchDaemon (system-wide, restart-on-fail) |

## Registering a new tunnel (first time, or rotation)

```bash
# 1. Authenticate cloudflared (one-time per machine; opens a browser).
cloudflared tunnel login

# 2. Create the tunnel.
cloudflared tunnel create owera-operator-plane
# -> prints the UUID and the credentials-file path
#    typically: ~/.cloudflared/<UUID>.json
#    OUTPUT: "Created tunnel owera-operator-plane with id <UUID>"

# 3. Move credentials to the daemon directory with the right ownership.
sudo mkdir -p /usr/local/etc/cloudflared
sudo mv ~/.cloudflared/<UUID>.json /usr/local/etc/cloudflared/
sudo chown _cloudflared:_cloudflared /usr/local/etc/cloudflared/<UUID>.json
sudo chmod 600 /usr/local/etc/cloudflared/<UUID>.json

# 4. Render config.yml from the example, substituting the UUID.
sudo cp tunnel/config.example.yml /usr/local/etc/cloudflared/config.yml
sudo sed -i '' "s|{{REFERENCE:cloudflared.tunnel.owera-operator-plane.uuid}}|<UUID>|g" \
  /usr/local/etc/cloudflared/config.yml
sudo chown root:_cloudflared /usr/local/etc/cloudflared/config.yml
sudo chmod 640 /usr/local/etc/cloudflared/config.yml

# 5. Route the hostname to the tunnel (creates the CNAME in Cloudflare).
cloudflared tunnel route dns owera-operator-plane internal-rpc.owera.ai

# 6. Install the LaunchDaemon.
sudo cp tunnel/service.example.plist /Library/LaunchDaemons/ai.owera.cloudflared.plist
# Render template vars in-place (or use fleetctl render — TODO):
sudo sed -i '' "s|{{.Label}}|ai.owera.cloudflared|g; \
                s|{{.BinPath}}|$(which cloudflared)|g; \
                s|{{.ConfigPath}}|/usr/local/etc/cloudflared/config.yml|g; \
                s|{{.HomeDir}}|/var/log/cloudflared|g" \
  /Library/LaunchDaemons/ai.owera.cloudflared.plist
sudo mkdir -p /var/log/cloudflared
sudo chown _cloudflared:_cloudflared /var/log/cloudflared

# 7. Bootstrap.
sudo launchctl bootstrap system /Library/LaunchDaemons/ai.owera.cloudflared.plist
sudo launchctl enable system/ai.owera.cloudflared

# 8. Verify.
sudo launchctl print system/ai.owera.cloudflared | grep state
curl -sS https://internal-rpc.owera.ai/healthz   # expect 200 from the operator plane
```

## Rotation

cloudflared tunnel credentials should rotate annually (see `../infra/secrets-manifest.md`). Procedure:

1. Provision a v2 tunnel: `cloudflared tunnel create owera-operator-plane-v2`.
2. Repoint DNS: `cloudflared tunnel route dns owera-operator-plane-v2 internal-rpc.owera.ai` (overrides existing CNAME).
3. Update `config.yml` with the new UUID and credentials path.
4. `sudo launchctl kickstart -k system/ai.owera.cloudflared` to reload.
5. Verify health, then `cloudflared tunnel delete owera-operator-plane` (old).
6. Update `../infra/dns.cloudflare.yaml` and `../infra/secrets-manifest.md` in the same commit.

## Logs

- `/var/log/cloudflared/cloudflared.out.log` — daemon stdout
- `/var/log/cloudflared/cloudflared.err.log` — daemon stderr
- `launchctl print system/ai.owera.cloudflared` — runtime state, last exit, throttle status

## Health probe

cloudflared exposes Prometheus-style metrics on `127.0.0.1:9300`. Useful one-liners:

```bash
curl -s http://127.0.0.1:9300/metrics | grep cloudflared_tunnel_ha_connections
curl -s http://127.0.0.1:9300/ready
```

A failed-open tunnel returns `cloudflared_tunnel_ha_connections 0` for >30s — the operator-plane watchdog (see `owera-fleet`) treats that as a Sev2.
