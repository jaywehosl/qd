# qd

A QUIC tunnel: nodes, a panel, clients for Windows and Android.

Traffic rides QUIC — TCP flows as CONNECT streams, UDP and ICMP as connect-ip
datagrams. On every other path a node serves an ordinary website, so from the
outside it is HTTPS and nothing else.

## Requirements

A node needs a domain pointing at it, ports 443/udp and 443/tcp free, and
80/tcp free while the certificate is issued. Debian or Ubuntu with systemd,
x86-64.

## Starting a network

```
bash <(curl -Ls https://raw.githubusercontent.com/jaywehosl/qd/main/install-node.sh)
```

It asks for the domain, an e-mail for Let's Encrypt, the udp port, and the tags
of the first administrator and the first group. Everything else comes from
defaults you can change later in the panel.

The script checks the DNS records against this machine, checks the ports,
installs certbot, issues the certificate with a dry run first, writes the
renewal hook, starts the node, and only prints the administrator link once the
node answers on udp, serves the site on tcp and replies to a control request.

Import the printed `qd://` link into a client. The same link opens the panel.

## Adding a node

In the panel: Nodes → Add node → fill in the domain, port and role → Generate
deploy script. Run the printed command on the new machine; it needs nothing
typed by hand.

## Clients

Windows: run `qd-client-windows-amd64.exe` as administrator, import the link.

Android: install `qd-android-arm64.apk`, import the link.

## Node commands

```
qd-node -help          what it can be asked to do
qd-node -status        what this node is, whether it serves, what it sees
qd-node -restart       restart the service
qd-node -admins        list administrators
qd-node -admin-add TAG add an administrator, print its link
qd-node -cert-hook     rewrite the certbot renewal hook
```

`-admin-add` is the way back in when the administrator link is lost: it works
on the machine itself, because over the network a node answers no one but an
administrator.

## Updating

```
bash <(curl -Ls https://raw.githubusercontent.com/jaywehosl/qd/main/install-node.sh) --mode update
```

Keeps the database, swaps the binary, rolls back if the new one does not start.

## Building

```
sh scripts/setup-third-party.sh
go build ./cmd/qd-node
```

The patched quic-go and connect-ip-go are not kept in the repository, only the
patches; the script restores them. The Windows client additionally needs
WinDivert and wintun binaries, which are not redistributed here.
