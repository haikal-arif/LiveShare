# The one public entry point: puts the frontend, backend and SFU behind a
# single origin. The config is a template; the official nginx image fills
# in GATEWAY_* variables at start (see NGINX_ENVSUBST_FILTER) and writes it
# to /etc/nginx/conf.d/. TLS certificates are mounted, never baked in.
FROM nginx:1.30.3-alpine

ENV NGINX_ENVSUBST_FILTER=^GATEWAY_ \
    GATEWAY_HTTPS_PORT=443

COPY templates/ /etc/nginx/templates/
COPY snippets/ /etc/nginx/snippets/
# Entrypoint hooks run before nginx starts. The blocklist watcher must be
# executable, so set the mode explicitly rather than trust the checkout.
COPY --chmod=0755 docker-entrypoint.d/ /docker-entrypoint.d/

EXPOSE 80 443
