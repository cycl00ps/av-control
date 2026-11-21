# Integra DRX-3.4 remote

Go + HTMX LAN remote for the Integra AVR over eISCP. UX reference: [`mockup/`](mockup/). Design notes: [`PLAN.md`](PLAN.md), [`CAPABILITIES.md`](CAPABILITIES.md), [`PROTOCOL.md`](PROTOCOL.md).

## Config

Copy [`.env.example`](.env.example) to `.env` and set values. `.env` is gitignored.

| Variable | Meaning |
|----------|---------|
| `INTEGRA_ADDRESS` | Amp IP |
| `INTEGRA_EISCP_PORT` | Default `60128` |
| `INTEGRA_MOCK` | `1` = in-memory fake amp (no network to AVR) |
| `INTEGRA_HTTP_ADDR` | Listen address, e.g. `0.0.0.0:8080` |
| `listening_mode` / `master_volume` / `center_level` / `late_night` | Apply-defaults prefs |

## Run

```bash
cd Integra
cp .env.example .env   # edit as needed
go run ./cmd/integra-web
```

Static mockup only (no Go):

```bash
cd mockup && python3 -m http.server 8090 --bind 0.0.0.0
```

## CLI helper

[`eiscp_send.py`](eiscp_send.py) — ad-hoc eISCP from a host that can reach the amp.

## Run with Docker Compose

Use a Docker host on the LAN with Docker Engine, Compose, and Buildx.
Check the plugins with `docker compose version` and `docker buildx version`.
On CachyOS or Arch, install a missing Buildx plugin with
`sudo pacman -S docker-buildx`. Run Docker commands as the user who owns
the Docker context, especially with rootless Docker.

From the repository root:

```sh
cd Integra
test -f .env || cp .env.example .env
```

Set `INTEGRA_ADDRESS` in `.env` to your receiver's LAN IP and keep
`INTEGRA_MOCK=0` for live mode. The Docker host needs outbound access to
the receiver's eISCP TCP port, `60128` by default.

```sh
docker login dhi.io
docker compose build
docker compose up -d
docker compose ps
docker compose logs integra-web
curl -fsS http://127.0.0.1:8080/healthz
```

The [Dockerfile](Dockerfile) compiles Go in a Docker Hardened Images
(DHI) build stage and ships the binary and UI assets in a non-root DHI
Static runtime. The health check returns `ok` when the HTTP process
responds. To verify receiver connectivity in live mode, compare the
power and input shown in the UI with the receiver's current state.

### Try it without a receiver

From `Integra/`, start an isolated mock service and open
`http://127.0.0.1:18080` on the Docker host:

```sh
INTEGRA_MOCK=1 INTEGRA_HTTP_BASE= INTEGRA_HOST_PORT=18080 \
  docker compose -p integra-smoke up -d --build
curl -fsS http://127.0.0.1:18080/healthz
```

Remove the test service and its test volume when finished:

```sh
docker compose -p integra-smoke down -v
```

### Access from another device with Caddy

Compose publishes HTTP on the Docker host's loopback interface. Open
`http://127.0.0.1:8080` directly on that host when serving at the site
root. For access from a phone or another LAN device, put Caddy on the
Docker host and have it forward requests to `127.0.0.1:8080`. If you
change `INTEGRA_HOST_PORT` in `.env`, use the same port in the upstream
and local checks. The container still listens on port `8080`.

For `https://av.home.arpa/av/`, create a LAN DNS record pointing
`av.home.arpa` at the Docker host. Set `INTEGRA_HTTP_BASE=/av` in
`.env` and apply it with `docker compose up -d`. Add this site to the
host's Caddyfile:

```caddyfile
av.home.arpa {
    tls internal
    redir /av /av/ 308
    handle_path /av/* {
        reverse_proxy 127.0.0.1:8080
    }
}
```

The app adds `/av` to browser URLs, and Caddy strips it before forwarding
requests. For a dedicated hostname served at `/`, leave
`INTEGRA_HTTP_BASE` empty and put `reverse_proxy 127.0.0.1:8080` directly
in the site block.

With `tls internal`, each client must trust Caddy's local CA certificate.
Keep access within a trusted LAN because the receiver controls have no
built-in authentication. [Caddy local HTTPS](https://caddyserver.com/docs/automatic-https#local-https)

For an existing systemd-managed Caddy service using the default config path:

```sh
sudo caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
sudo systemctl reload caddy
```

From a LAN client that trusts the CA, open `https://av.home.arpa/av/`
and verify the redirect, health endpoint, CSS, and HTMX partial:

```sh
curl -sSI https://av.home.arpa/av
curl -fsS https://av.home.arpa/av/healthz
curl -fsS -o /dev/null https://av.home.arpa/av/static/app.css
curl -fsS -o /dev/null https://av.home.arpa/av/partials/remote
```

### Update or stop

After updating the checkout, refresh the base images and recreate the
service as needed:

```sh
docker compose build --pull
docker compose up -d
```

`docker compose down` stops the service and preserves TuneIn pins in
the `integra-data` named volume. `docker compose down -v` also deletes
the volume and its pins.
