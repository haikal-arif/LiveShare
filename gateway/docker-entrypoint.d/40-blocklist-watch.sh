#!/bin/sh
# Enforces the room blacklist at the gateway.
#
# The webserver writes BLOCKLIST_FILE (mounted here at
# /etc/nginx/blocklist) with one "deny <ip>;" per blocked address. This
# loop checks the file once a second and, when its contents change, runs
# `nginx -t` and then `nginx -s reload`. A reload re-reads the config
# without dropping open connections; a restart would cut every watcher's
# video and every signaling socket.
#
# The official nginx entrypoint runs this before exec'ing nginx, so the
# loop is orphaned into nginx's process tree and keeps running with it.
# It stops when the container does.

BLOCKLIST=/etc/nginx/blocklist/blocklist.conf
INTERVAL=1

# nginx refuses to start without the file the include points at. The
# webserver rewrites it at its own startup, so empty is fine here.
if [ ! -f "$BLOCKLIST" ]; then
    : > "$BLOCKLIST"
fi

(
    last=$(cksum < "$BLOCKLIST")
    while sleep "$INTERVAL"; do
        now=$(cksum < "$BLOCKLIST" 2>/dev/null || echo missing)
        if [ "$now" != "$last" ]; then
            last=$now
            # A bad config fails `nginx -t`, so the running config stays
            # in place. The watcher waits for the next change before it
            # retries.
            if nginx -t && nginx -s reload; then
                echo "blocklist: reloaded nginx"
            else
                echo "blocklist: reload failed, keeping the previous config" >&2
            fi
        fi
    done
) &
