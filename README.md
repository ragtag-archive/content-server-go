# content-server-go

Emergency Go rewrite of Ragtag Archive's original
[archive-content-server](https://gitlab.com/aonahara/archive-content-server),
because it kept hanging when it received too many requests.

Requests are served from the upstreams listed in the overrides file. A request
for `/<upstream tag>/<path>` is either redirected or proxied to
`<upstream URL>/<path>`, depending on the "must proxy" column. Requests for
any other upstream tag return 404.

## Configuration

```
LISTEN_ADDRESS=127.0.0.1:8080
OVERRIDES_FILE=overrides.tsv
```

The overrides file is a tab-separated file with the columns `UPSTREAM TAG`,
`UPSTREAM URL` and `MUST PROXY`. See [overrides.example.tsv](overrides.example.tsv).
Lines that don't have exactly three columns are ignored.

## Deployment

Prebuilt `linux/amd64` and `linux/arm64` binaries are uploaded as artifacts
by the [build workflow](.github/workflows/ci.yml) on every push to `main`.

```sh
# Build the project
go build

# Move the binary
install -m 0755 content-server-go /usr/local/bin/content-server-go

# Create systemd service
cat <<EOT | sudo tee /etc/systemd/system/content-server-go.service
[Unit]
Description=Content Server
After=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/content-server-go
Restart=on-failure
Environment="LISTEN_ADDRESS=127.0.0.1:8080"
Environment="OVERRIDES_FILE=/etc/content-server-go/overrides.tsv"

[Install]
WantedBy=multi-user.target
EOT

# Enable and run the service
systemctl enable --now content-server-go.service
```
