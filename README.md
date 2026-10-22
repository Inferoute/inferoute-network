# inferoute-network

Headscale fork that runs more than one tailnet in a single process. A cluster is a network. An Inferoute user is a Headscale user inside that network. Their machine is a node joined with a single-use pre-auth key.

Nodes in one network can ping each other. Nodes in another network do not see them.

## Test on a server

One Ubuntu machine runs Headscale. Two other machines run the normal Tailscale client. Nothing else is required. Skip Traefik for this test.

On the server:

```shell
go build -o headscale ./cmd/headscale
mkdir -p /var/lib/headscale
```

Write `/etc/headscale/config.yaml`. `server_url` must be an address the other machines can reach. `127.0.0.1` makes them talk to themselves.

```yaml
server_url: http://203.0.113.10:8080
listen_addr: 0.0.0.0:8080
metrics_listen_addr: 127.0.0.1:9090
unix_socket: /var/lib/headscale/headscale.sock
noise:
  private_key_path: /var/lib/headscale/noise_private.key
prefixes:
  v4: 100.64.0.0/10
  v6: fd7a:115c:a1e0::/48
database:
  type: sqlite
  sqlite:
    path: /var/lib/headscale/db.sqlite
dns:
  magic_dns: true
  base_domain: tailnet.inferoute.internal
  override_local_dns: true
  nameservers:
    global:
      - 1.1.1.1
derp:
  server:
    enabled: false
  urls:
    - https://controlplane.tailscale.com/derpmap/default
  auto_update_enabled: false
policy:
  mode: file
  path: ""
```

On the Headscale server, allow inbound TCP `8080`. The client machines only connect out to it. Then start Headscale:

```shell
./headscale serve
```

In another shell on the same server, create two clusters and one key per machine. `--user` is the Inferoute username. The same name in two clusters is allowed.

```shell
./headscale networks create --name cluster-a
./headscale preauthkeys create --network cluster-a --user alice --expiration 24h
./headscale preauthkeys create --network cluster-a --user bob --expiration 24h

./headscale networks create --name cluster-b
./headscale preauthkeys create --network cluster-b --user carol --expiration 24h
```

On Alice's machine and Bob's machine:

```shell
tailscale up --login-server=http://203.0.113.10:8080 --auth-key=<their-key> --accept-routes
```

On Carol's machine, use the `cluster-b` key.

Alice and Bob each run `tailscale status`. Each should list the other and nobody else. `tailscale ip -4` prints a `100.64.x.x` address. `tailscale ping` to that address should succeed.

Carol's `tailscale status` must not list Alice or Bob. `tailscale ping` to one of their addresses must fail with `no matching peer`.

If `tailscale status` shows the peer and the ping still fails, the cluster is fine and UDP `41641` between those two machines is not. If the peer is missing, the control server is the problem.

## Test in Docker

This runs real Tailscale clients and checks the same isolation. It needs Docker.

```shell
go run ./cmd/hi doctor
go run ./cmd/hi run "^TestTwoIsolatedNetworks$" --timeout=20m
```

After this branch is on `main`, GitHub publishes `ghcr.io/inferoute/inferoute-network:edge`. `deploy/` is the later layout, with Traefik and this machine's own DERP relay. Do not use that config for the test above.
