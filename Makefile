# Runs the three components: Rust SFU, Go webserver, SvelteKit frontend.
#
# Run `make` or `make help` to list the targets (built from the ## comments).
#
# The webserver reads ROOM_PASSCODE, JWT_SECRET and SFU_BASE_URL from
# capstone_be/.env (loaded below if present) or from the environment.

BE_DIR  := capstone_be
FE_DIR  := capstone_fe
SFU_DIR := capstone_sfu

ifneq (,$(wildcard $(BE_DIR)/.env))
include $(BE_DIR)/.env
export
endif

.DEFAULT_GOAL := help

.PHONY: help setup be-env sfu-env fe-env dev sfu be fe install build test coverage clean \
	env-file docker-setup docker-build docker-up docker-down docker-logs tls run-all

help: ## Show this list
	@echo "Usage: make <target>"
	@echo
	@grep -hE '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN { FS = ":.*## " } { printf "  %-16s %s\n", $$1, $$2 }'

# Ports shared between the generated .env files.
SFU_PORT ?= 9000
BE_PORT  ?= 8080
FE_PORT  ?= 5173

setup: be-env sfu-env fe-env install ## Create a .env in each component (never overwrites)

# Existing .env files are left untouched; delete one to regenerate it.
be-env: $(BE_DIR)/.env
sfu-env: $(SFU_DIR)/.env
fe-env: $(FE_DIR)/.env

$(BE_DIR)/.env:
	@printf '%s\n' \
		'ADDR=:$(BE_PORT)' \
		'ROOM_PASSCODE=changeme' \
		'JWT_SECRET=$(shell openssl rand -hex 32)' \
		'SFU_BASE_URL=http://127.0.0.1:$(SFU_PORT)' \
		'SFU_WS_URL=ws://127.0.0.1:$(SFU_PORT)/ws' \
		'ALLOWED_ORIGINS=http://localhost:$(FE_PORT)' \
		'TRUSTED_PROXIES=127.0.0.1,::1' \
		'ICE_SERVERS=stun:stun.l.google.com:19302' > $@
	@echo "created $@ (change ROOM_PASSCODE)"

# The SFU and frontend don't read any env yet; these hold the agreed
# addresses so they stay in sync with the webserver's.
$(SFU_DIR)/.env:
	@printf '%s\n' 'SFU_ADDR=127.0.0.1:$(SFU_PORT)' > $@
	@echo "created $@"

$(FE_DIR)/.env:
	@printf '%s\n' 'PUBLIC_API_URL=http://localhost:$(BE_PORT)' > $@
	@echo "created $@"

dev: ## Run all three together (Ctrl-C stops all)
	@trap 'kill 0' INT TERM EXIT; \
	$(MAKE) --no-print-directory sfu & \
	$(MAKE) --no-print-directory be & \
	$(MAKE) --no-print-directory fe & \
	wait

sfu: ## Run the SFU (cargo run)
	cd $(SFU_DIR) && cargo run

be: ## Run the backend (go run)
	cd $(BE_DIR) && go run ./cmd/webserver

fe: $(FE_DIR)/node_modules ## Run the frontend dev server
	cd $(FE_DIR) && npm run dev

install: $(FE_DIR)/node_modules

$(FE_DIR)/node_modules: $(FE_DIR)/package-lock.json
	cd $(FE_DIR) && npm ci
	@touch $@

build: ## Build all three
	cd $(SFU_DIR) && cargo build --release
	cd $(BE_DIR) && go build -o bin/webserver ./cmd/webserver
	cd $(FE_DIR) && npm run build

test: ## Run backend, SFU and frontend unit tests
	cd $(BE_DIR) && go test ./...
	cd $(SFU_DIR) && cargo test
	cd $(FE_DIR) && npm run test:unit -- --run

coverage: ## SFU line coverage: HTML report plus a per-file summary
	cd $(SFU_DIR) && cargo llvm-cov --workspace --html
	cd $(SFU_DIR) && cargo llvm-cov report --summary-only
	@echo "report: $(SFU_DIR)/target/llvm-cov/html/index.html"

clean: ## Remove build outputs
	rm -rf $(BE_DIR)/bin $(FE_DIR)/.svelte-kit $(FE_DIR)/build
	cd $(SFU_DIR) && cargo clean

# ---- Docker: the components behind an nginx gateway (see EC2-DEPLOY.md) ----
# Options are given on the command line and update .env. Leave one out to
# keep what .env already has.
#   HOST_IP=1.2.3.4          public address clients reach (APP_HOST_IP)
#   EC2=1                    take HOST_IP from the EC2 instance metadata
#   DOMAIN=name.duckdns.org  hostname for links and the certificate
#   HTTPS_PORT=443           gateway HTTPS port on the host (default 8443)
#   HTTP_PORT=80             gateway HTTP redirect port (default 8088)
#   TURN=1 | TURN=0          run the TURN relay (coturn) or not
#   EMAIL=you@example.com    Let's Encrypt contact, for `make tls`
#
# EC2 example:
#   make docker-setup EC2=1 DOMAIN=flashlive.duckdns.org HTTPS_PORT=443 HTTP_PORT=80 TURN=1
#   make tls DOMAIN=flashlive.duckdns.org EMAIL=you@example.com
#   make docker-up

ENV_OPTS = HOST_IP=$(HOST_IP) EC2=$(EC2) DOMAIN=$(DOMAIN) HTTPS_PORT=$(HTTPS_PORT) \
	HTTP_PORT=$(HTTP_PORT) TURN=$(TURN)

env-file: ## Create or update .env from the options (keeps existing values)
	@./scripts/docker-env.sh $(ENV_OPTS)

docker-setup: env-file gateway/certs/capstone.crt ## Write .env and the gateway's TLS cert (mkcert locally)

# Existing files are left untouched; delete one to regenerate it.
gateway/certs/capstone.crt:
	./gateway/gen-cert.sh

docker-build: docker-setup ## Build the images only
	docker compose build

docker-up: docker-setup ## Build the images and run the stack
	docker compose up -d --build
	@echo "up: https://$$(grep ^APP_PUBLIC_HOST= .env | cut -d= -f2):$$(grep ^APP_HTTPS_PORT= .env | cut -d= -f2)"
	@grep -q '^COMPOSE_PROFILES=turn' .env && echo "turn: running (see APP_TURN_URLS in .env)" || echo "turn: off (TURN=1 to enable)"

docker-down: ## Stop and remove the stack
	docker compose down

docker-logs: ## Follow the stack's logs
	docker compose logs -f

# Let's Encrypt certificate for DOMAIN, copied over the gateway's cert (and
# TURN's). Needs DOMAIN, EMAIL, certbot, and port 80 open to the internet.
# Run again before it expires (about every 60 days) to renew.
tls: env-file ## Get a Let's Encrypt cert for DOMAIN (EC2; needs EMAIL=)
	@set -e; \
	[ -n "$(EMAIL)" ] || { echo "EMAIL=you@example.com is required" >&2; exit 1; }; \
	command -v certbot >/dev/null || { echo "certbot is not installed (see EC2-DEPLOY.md)" >&2; exit 1; }; \
	domain=$$(grep ^APP_PUBLIC_HOST= .env | cut -d= -f2); \
	case "$$domain" in *[a-zA-Z]*) ;; *) echo "DOMAIN=name.example.com is required (an IP address cannot get a Let's Encrypt cert)" >&2; exit 1;; esac; \
	port=$$(grep ^APP_HTTPS_PORT= .env | cut -d= -f2); \
	trap 'docker compose up -d gateway >/dev/null 2>&1 || true' EXIT; \
	docker compose stop gateway >/dev/null 2>&1 || true; \
	sudo certbot certonly --standalone --non-interactive --agree-tos --keep-until-expiring \
		-m "$(EMAIL)" -d "$$domain"; \
	sudo cp "/etc/letsencrypt/live/$$domain/fullchain.pem" gateway/certs/capstone.crt; \
	sudo cp "/etc/letsencrypt/live/$$domain/privkey.pem" gateway/certs/capstone.key; \
	sudo chmod 644 gateway/certs/capstone.crt gateway/certs/capstone.key; \
	trap - EXIT; \
	docker compose up -d --build gateway; \
	echo "tls: certificate for $$domain installed; https://$$domain:$$port"

# Full LAN run. The IP is re-detected and the cert regenerated on every run,
# since both depend on the current network. Ctrl-C (SIGINT/SIGTERM), a failed
# step, or the logs ending all go through cleanup: it first stops the SFU so
# it tells every open room `room_closed` while the gateway is still up, then
# tears the stack down and closes the firewall.
run-all: env-file ## Detect LAN IP, refresh cert, open firewall, up, attach to logs (Ctrl-C tears down)
	@set -e; \
	ip=$$(ip route get 1.1.1.1 | awk '{for(i=1;i<=NF;i++) if($$i=="src") print $$(i+1)}'); \
	[ -n "$$ip" ] || { echo "could not detect LAN IP" >&2; exit 1; }; \
	echo "LAN IP: $$ip"; \
	if grep -q '^APP_HOST_IP=' .env; then \
		sed -i "s/^APP_HOST_IP=.*/APP_HOST_IP=$$ip/" .env; \
	else \
		echo "APP_HOST_IP=$$ip" >> .env; \
	fi; \
	./gateway/gen-cert.sh; \
	cleanup() { \
		trap - EXIT; \
		echo; \
		echo "Shutting down..."; \
		docker compose stop sfu || echo "sfu stop failed"; \
		docker compose down || echo "compose down failed"; \
		./firewall.sh close; \
	}; \
	trap cleanup EXIT; \
	trap 'exit 130' INT; \
	trap 'exit 143' TERM; \
	./firewall.sh open; \
	docker compose up -d --build; \
	docker compose restart gateway; \
	echo "up: https://$$ip:$$(grep ^APP_HTTPS_PORT= .env | cut -d= -f2)"; \
	docker compose logs -f
