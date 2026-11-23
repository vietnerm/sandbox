#!/usr/bin/env bash
# alpine-nginx allowlist workload — the D45 stack measured as the payload sees
# it: spawn-fcgi + sqlite3 seed + nginx foreground (PID 2), traffic generated
# INSIDE the instance (busybox wget hits 127.0.0.1:<p>) so only the payload
# tree is traced — pasta/webproxy are host-side siblings, correctly excluded.
# Base = rootfs-alpine copy (has /bin/sh busybox + nginx/fcgiwrap/sqlite3
# preinstalled --no-scripts, D45), files written per sandbox.sh write_stack_files.
REPO="$(cd "$(dirname "$0")/../.." && pwd)"
PROBE="${PROBE:-$REPO/tools/syscall-probe}"
RAW="${RAW:-/tmp/probe-raw.txt}"
P=18089
R=$(mktemp -d)
cp -a rootfs-alpine/. "$R/"
mkdir -p "$R/run" "$R/etc/nginx" "$R/var/log/nginx" "$R/var/lib/minict" \
         "$R/www/cgi-bin" "$R/var/lib/nginx/body" "$R/var/lib/nginx/proxy" \
         "$R/var/lib/nginx/fastcgi" "$R/var/tmp/nginx"
cat > "$R/etc/nginx/nginx.conf" <<NGINX
user root;
pid /run/nginx.pid;
worker_processes 1;
error_log /var/log/nginx/error.log warn;
events { worker_connections 64; }
http {
    include /etc/nginx/mime.types;
    access_log /var/log/nginx/access.log;
    client_body_temp_path /var/lib/nginx/body;
    proxy_temp_path /var/lib/nginx/proxy;
    fastcgi_temp_path /var/lib/nginx/fastcgi;
    uwsgi_temp_path /var/tmp/nginx;
    scgi_temp_path /var/lib/nginx;
    server { listen 127.0.0.1:$P;
        location / { root /www; index index.html; }
        location /api { include /etc/nginx/fastcgi_params; fastcgi_pass unix:/run/fcgi.sock; }
    }
}
NGINX
echo '<h1>ok</h1>' > "$R/www/index.html"
cat > "$R/www/cgi-bin/api.cgi" <<'CGI'
#!/bin/sh
printf 'Content-Type: application/json\r\n\r\n'
if [ "$REQUEST_METHOD" = "POST" ]; then
  sqlite3 /var/lib/minict/app.sqlite "INSERT INTO items(name) VALUES('probe');"
  echo '{"ok":true}'
else
  sqlite3 /var/lib/minict/app.sqlite 'SELECT json_array(id,name) FROM items;' | tr -d '\n'
  echo
fi
CGI
chmod +x "$R/www/cgi-bin/api.cgi"
PAYLOAD='spawn-fcgi -s /run/fcgi.sock -M 0666 -- /usr/bin/fcgiwrap || exit 1;
  mkdir -p /var/lib/minict;
  sqlite3 /var/lib/minict/app.sqlite "CREATE TABLE IF NOT EXISTS items(id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, created TEXT DEFAULT CURRENT_TIMESTAMP);" || exit 1;
  /usr/sbin/nginx -c /etc/nginx/nginx.conf & n=$!;
  sleep 0.5;
  wget -qO- http://127.0.0.1:18089/ >/dev/null || echo IDX_FAIL;
  wget -qO- http://127.0.0.1:18089/api/health >/dev/null || echo API_FAIL;
  wget -qO- --post-data=name=probe http://127.0.0.1:18089/api/items >/dev/null || echo POST_FAIL;
  wget -qO- http://127.0.0.1:18089/ >/dev/null || echo IDX2_FAIL;
  echo TRAFFIC_OK; kill $n 2>/dev/null'
env MINICT_WEB_PORT="$P" MINICT_PROXY_SOCK="/.minict-proxy.sock" \
  "$PROBE" --after-exec 2 --append "$RAW" -- sandbox/minict "$R" /bin/sh -c "$PAYLOAD"
rm -rf "$R"
