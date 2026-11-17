# Deploying FlashLive on EC2

This runs the whole stack (web app, backend, SFU, gateway and the TURN relay) on one EC2 instance, reachable from phones on mobile data as well as Wi-Fi.

The work has two halves:

1. **Create the server.** Terraform creates the instance, its firewall rules and a fixed public address (section 2). The same thing can be done by hand in the AWS console (section 3).
2. **Install and start the app on it.** `make` targets write the `.env` file, get the TLS certificate and start the stack with `docker compose` (sections 4 to 9).

## 1. What you need

- An AWS account, and credentials on your laptop that Terraform can use (for example `AWS_PROFILE`, or the `AWS_ACCESS_KEY_ID` variables).
- Terraform 1.5 or newer.
- An EC2 **key pair** in the region you deploy to. The Terraform config expects one named `LiveShare` (private key `LiveShare.pem`). Key pairs are per region.
- A free DuckDNS subdomain, such as `flashlive.duckdns.org`.
- An email address, for the Let's Encrypt certificate.
- This repository on your laptop.

## 2. Create the server with Terraform

The config is `terraform/ec2-deploy/main.tf`. It creates:

- a security group with the inbound rules in the table below,
- one `c7i-flex.large` instance (x86, 2 vCPU, 4 GB) with Ubuntu Server 24.04 LTS and a 30 GB gp3 disk,
- an Elastic IP attached to the instance, so the address stays the same across restarts.

It uses the region's default VPC.

### 2.1 Apply

```bash
cd terraform/ec2-deploy
terraform init

# Port 22 is opened only to this address. Use your own public IP.
MY_IP=$(curl -s https://checkip.amazonaws.com)

terraform plan -var "ssh_cidr=${MY_IP}/32" -out plan
terraform apply plan
```

Read the plan before applying it. A first run should show 4 resources to add (security group, instance, Elastic IP, association) and nothing to change or destroy.

When it finishes, Terraform prints two outputs:

```bash
terraform output elastic_ip     # the public address, used in the next steps
terraform output ssh_command    # ssh -i ~/keys/LiveShare.pem ubuntu@<ELASTIC_IP>
```

### 2.2 Variables

Only `ssh_cidr` is required. Pass others with more `-var` flags.

| Variable | Default | Meaning |
| --- | --- | --- |
| `ssh_cidr` | none (required) | Who may SSH in, for example `203.0.113.10/32`. |
| `region` | `ap-southeast-2` (Sydney) | Region to deploy into. The account's organization policy denied EC2 in `ap-southeast-1`, so the default is Sydney. |
| `instance_type` | `c7i-flex.large` | Use `t3.medium` if this type isn't offered in the region. Smaller types run out of memory during the Rust and Go builds. |
| `key_name` | `LiveShare` | An existing key pair in that region. |
| `ami_id` | latest Ubuntu 24.04 (looked up) | Set only to pin a specific image. |
| `root_volume_size` | `30` | Disk size in GB. |
| `name` | `flashlive` | Name prefix for the resources. |

### 2.3 Inbound rules

| Port | Protocol | Source | Purpose |
| --- | --- | --- | --- |
| 22 | TCP | `ssh_cidr` | SSH |
| 80 | TCP | 0.0.0.0/0 | Let's Encrypt validation and HTTP redirect |
| 443 | TCP | 0.0.0.0/0 | Web app, API and signaling (`wss`) |
| 3478 | TCP and UDP | 0.0.0.0/0 | TURN |
| 5349 | TCP | 0.0.0.0/0 | TURN over TLS |
| 49152–49200 | UDP | 0.0.0.0/0 | TURN relay media |
| 50000–50199 | UDP | 0.0.0.0/0 | SFU media (WebRTC) |

Ports 8080, 9000 and 5173 stay closed. Those services are internal to Docker, and the gateway is the only way in. Each TURN relay and each SFU connection uses its own UDP port, so the two ranges must stay open to all clients.

### 2.4 Things to know about the Terraform state

- **The state is a local file** (`terraform/ec2-deploy/terraform.tfstate`). It's how Terraform knows which AWS resources it owns. Don't delete it while the server exists, and don't commit it: it holds account and resource IDs. `.gitignore` excludes the state, the saved plans and `.terraform/`.
- **Your IP changes.** If SSH starts timing out, run the plan and apply again with the new address. Only the SSH rule changes.
- **Changing region.** Destroy the old deployment first, passing the old region explicitly (section 11). Running `apply` or `destroy` against a different region than the one the state was created in makes Terraform forget the old resources without deleting them, and they keep costing money.
- **A newer Ubuntu image doesn't replace the instance.** The config ignores AMI changes on purpose, so a later `apply` never wipes a running server.

## 3. Or create the server by hand

Skip this if you used Terraform.

1. **Launch an EC2 instance** in the EC2 console: Ubuntu Server 24.04 LTS, `c7i-flex.large` or `t3.medium`, 30 GB gp3, and your key pair.
2. **Allocate an Elastic IP** (EC2 → Elastic IPs → Allocate) and associate it with the instance.
3. **Security group:** add the inbound rules from the table in section 2.3.

## 4. Get a DuckDNS name

1. Go to [duckdns.org](https://www.duckdns.org) and sign in.
2. Under **domains**, add a subdomain such as `flashlive`, and set its IP to the **Elastic IP**.
3. Check that it resolves from your laptop:

   ```bash
   dig +short flashlive.duckdns.org
   ```

   It should print the Elastic IP. Wait a few minutes if it doesn't.

## 5. Install the tools on the instance

Connect over SSH (the command is the `ssh_command` output):

```bash
ssh -i ~/keys/LiveShare.pem ubuntu@<ELASTIC_IP>
```

Then install Docker, `make`, `certbot` and the basics:

```bash
sudo apt-get update
sudo apt-get install -y make curl openssl certbot rsync
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker "$USER"
newgrp docker            # or log out and back in
docker compose version   # should print a v2 version
```

## 6. Copy the project

From the repository root on your laptop. Leave out your local `.env`, the certificates, the Terraform folder and the build folders, since the server makes its own:

```bash
rsync -av -e "ssh -i ~/keys/LiveShare.pem" \
  --exclude .env --exclude 'gateway/certs/*' \
  --exclude terraform/ \
  --exclude node_modules --exclude target --exclude .git \
  --exclude 'capstone_fe/build' --exclude '.svelte-kit' \
  ./ ubuntu@<ELASTIC_IP>:~/capstone/
```

Then on the instance:

```bash
cd ~/capstone
```

## 7. Configure

Set up `.env` for this server. The options update `.env`; anything you leave out keeps its current value:

```bash
make docker-setup EC2=1 DOMAIN=flashlive.duckdns.org HTTPS_PORT=443 HTTP_PORT=80 TURN=1
```

- `EC2=1` reads the public IP from the instance metadata, so you don't type it.
- `DOMAIN` is the DuckDNS name. It's used in the links and the certificate.
- `HTTPS_PORT=443` and `HTTP_PORT=80` are the standard ports, so people don't need to type a port.
- `TURN=1` starts the TURN relay. See [turn-relay.md](./turn-relay.md).

Then set the **room passcode**, and optionally a participant limit, in `.env`:

```bash
nano .env
```

- `APP_ROOM_PASSCODE`: the passcode anyone must enter to create a room. Don't leave it as `changeme`.
- `APP_MAX_PARTICIPANTS`: the most people per room (sharers and watchers together). `0` means no limit.

The JWT secret and the TURN password were generated by `make docker-setup`. Don't share `.env`.

## 8. Get the certificate

Let's Encrypt gives the gateway a certificate that phones and browsers trust, so there's no warning page:

```bash
make tls DOMAIN=flashlive.duckdns.org EMAIL=you@example.com
```

This needs port 80 open and the DuckDNS name resolving to the instance. It stops the gateway briefly while it validates, then starts it again with the new certificate.

Certificates expire after about 90 days. Run the same command again before that; it's safe to re-run at any time. When TURN is on, it also restarts the TURN relay so it loads the new certificate.

## 9. Start the stack

```bash
make docker-up
```

This builds the images and starts everything, including the TURN relay when `TURN=1` is set. The first build takes several minutes. The last lines show the address, and whether TURN is running.

Check that the services are up:

```bash
docker compose ps
docker compose logs --tail=50 sfu be turn
```

Open **https://flashlive.duckdns.org** in a browser. It should load with no warning.

## 10. Test it

1. **Laptop:** open the site, create a room with your passcode, and note the room code or QR code.
2. **Phone on mobile data** (Wi-Fi off): open the same address, join the room as a sharer, and check that the video shows up on the laptop.
3. **A second phone or laptop:** join as a watcher.
4. Check the participant limit: with `APP_MAX_PARTICIPANTS=2`, a third join should be refused with `room is full`.

If the video doesn't come through on mobile data, check the TURN logs (`docker compose logs turn`).

## 11. Tearing it down

On the instance, to stop the app but keep the server:

```bash
make docker-down
```

To delete the server, the security group and the Elastic IP, from your laptop:

```bash
cd terraform/ec2-deploy
terraform destroy -var 'region=ap-southeast-2' -var 'ssh_cidr=0.0.0.0/32'
```

Pass the region the deployment was created in. The `ssh_cidr` value doesn't matter for a destroy, but the variable has no default, so it has to be given. Everything on the instance is lost, including `.env` and the certificate.

A running instance is billed by the hour, and an Elastic IP is billed too. Destroy the deployment when the demo is over.

## Day-to-day commands

Run these on the instance, in `~/capstone`.

| Task | Command |
| --- | --- |
| Follow the logs | `make docker-logs` |
| Stop everything | `make docker-down` |
| Start again after a reboot | `make docker-up` (the containers also restart automatically) |
| Change the passcode or limit | Edit `.env`, then `make docker-up` |
| Turn TURN on or off | `make docker-setup TURN=1` (or `TURN=0`), then `make docker-up` |
| Renew the certificate | `make tls DOMAIN=flashlive.duckdns.org EMAIL=you@example.com` |
| Deploy a new version | Copy the folder again (section 6), then `make docker-up` |

Rooms and the blocklist live in memory, so they reset when the backend restarts.

## Troubleshooting

| Symptom | Likely cause and fix |
| --- | --- |
| `terraform apply` fails with an authorization or region error | The account may not allow EC2 in that region. Use the default region, or check with whoever manages the account. |
| `terraform apply` fails: key pair not found | The `LiveShare` key pair doesn't exist in that region. Create it there, or pass `-var 'key_name=<yours>'`. |
| SSH times out | Your public IP changed. Re-run plan and apply with the new `ssh_cidr`. |
| `make tls` fails: connection refused or timeout | Port 80 isn't open, or DuckDNS doesn't point at the Elastic IP yet. Check with `dig +short <domain>`. |
| The site loads but shows a certificate warning | `make tls` hasn't been run, or it failed. The self-signed certificate is the fallback. |
| The page loads, but no video shows on mobile data | TURN isn't reachable. Check ports 3478 (TCP and UDP), 5349 and 49152–49200 (UDP), and `make docker-setup TURN=1`. |
| Video works on Wi-Fi but not on mobile data | UDP 50000–50199 is blocked, or TURN is off. Turn TURN on and check the TURN logs. |
| `room is full` for a join | `APP_MAX_PARTICIPANTS` has been reached. Raise it in `.env` and run `make docker-up`, or set it to `0`. |
| `docker-setup` fails with `EC2 metadata not reachable` | Run it on the instance, not on your laptop. Or pass `HOST_IP=<public ip>` instead of `EC2=1`. |
| Build fails with out-of-memory | The instance is too small. Use `c7i-flex.large`, `t3.medium` or larger. |

## Security notes

- **Change the passcode** from `changeme` before you share the address.
- **TURN uses one shared username and password**, which the browser gets with the room data. That's fine for a demo.
- **The site uses HTTPS**, and TURN runs over TLS too.
- **Keep SSH limited** to your own IP (`ssh_cidr`).
- **`.env` and the Terraform state hold secrets and account details.** Don't commit them or copy them to shared places.
