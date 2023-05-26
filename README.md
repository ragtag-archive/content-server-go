# content-server-go

Emergency Go rewrite of Ragtag Archive's original
[archive-content-server](https://gitlab.com/aonahara/archive-content-server),
because it kept hanging when it received too many requests.

## Configuration

```
LISTEN_ADDRESS=0.0.0.0:8080
GD_DEFAULT_ROOT_ID=
GD_CLIENT_ID=
GD_CLIENT_SECRET=
GD_REFRESH_TOKEN=
```

## Deployment

```sh
# Build the project
go build

# Move the binary
install -m 0755 content-server-go /usr/local/bin/content-server-go

# Create systemd service
cat <<EOF | sudo tee /etc/systemd/system/content-server-go.service
[Unit]
Description=Content Server
After=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/content-server-go
Restart=on-failure
Environment="LISTEN_ADDRESS=0.0.0.0:8080"
Environment="GD_DEFAULT_ROOT_ID="
Environment="GD_CLIENT_ID="
Environment="GD_CLIENT_SECRET="
Environment="GD_REFRESH_TOKEN="

[Install]
WantedBy=multi-user.target
EOF

# Enable and run the service
systemctl enable --now content-server-go.service
```
