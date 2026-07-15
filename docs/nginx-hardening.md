# Nginx hardening: blocking scan-path 404 offenders

theia's access logs (and therefore its stats) pick up constant background noise from automated
scanners probing for common vulnerable paths: WordPress admin/login pages, exposed `.git`
directories, leaked credential files, and CGI traversal attempts. These are almost always 404s,
but they still cost a request/response cycle and pollute your traffic data. This note covers two
independent, composable defenses: nginx-level rate limiting, and fail2ban bans based on nginx's
own access log.

Both are optional and address the same problem from different angles — rate limiting slows down
any client hitting your server too fast (regardless of what they're requesting), while fail2ban
watches for the *specific* scan-path signatures below and bans the source IP outright after a few
hits. Using both is reasonable: rate limiting as a baseline, fail2ban for faster, targeted bans.

## 1. Deny common scan paths outright (nginx)

Add this to your `server {}` block (or an `/etc/nginx/snippets/theia-scan-block.conf` you
`include` from each site). `return 444` closes the connection without sending a response, which is
cheaper than a normal 404 and gives scanners nothing to work with:

```nginx
# /etc/nginx/snippets/theia-scan-block.conf
location ~* ^/(wp-admin|wp-login\.php|wp-includes|wp-content|xmlrpc\.php) {
    return 444;
}

location ~* \.(env|git/config|htaccess|htpasswd)$ {
    return 444;
}

location ~* ^/(sftp-config\.json|\.git|\.svn|\.hg)(/|$) {
    return 444;
}

location ~* ^/cgi-bin/ {
    return 444;
}

# Blind path traversal attempts (../, %2e%2e/, etc.) anywhere in the URI
if ($request_uri ~* "(\.\.%2f|%2e%2e/|\.\./)") {
    return 444;
}
```

Include it from each `server {}` block:

```nginx
server {
    ...
    include /etc/nginx/snippets/theia-scan-block.conf;
    ...
}
```

Then test and reload:

```bash
sudo nginx -t && sudo systemctl reload nginx
```

Note: requests that hit `return 444` still land in the access log (theia will still see and count
them as 404-equivalent noise unless you also set `access_log off;` on these locations), but the
scanner gets no response body and nginx does less work per request.

## 2. Rate limit repeat offenders (nginx)

`limit_req` throttles any client — scanner or not — that requests too quickly. Define a zone in
the `http {}` block of `nginx.conf`:

```nginx
http {
    ...
    limit_req_zone $binary_remote_addr zone=theia_scanlimit:10m rate=5r/m;
    ...
}
```

Then apply it to the same locations from step 1 (or globally in the `server {}` block):

```nginx
location ~* ^/(wp-admin|wp-login\.php|wp-includes|wp-content|xmlrpc\.php) {
    limit_req zone=theia_scanlimit burst=3 nodelay;
    return 444;
}
```

`rate=5r/m` allows 5 requests/minute per IP to matching paths before nginx starts rejecting with
503; `burst=3` allows a small burst above that before throttling kicks in.

## 3. Ban repeat offenders with fail2ban

fail2ban watches the nginx access log and bans (via iptables/nftables) IPs that trip a filter too
many times in a window. This is the most effective option against distributed but low-volume scans
that `limit_req` alone won't catch (each individual IP might only send a handful of requests).

Install fail2ban:

```bash
sudo apt-get install fail2ban   # Debian/Ubuntu
```

Create a filter matching the scan-path signatures theia's issue was raised for:

```ini
# /etc/fail2ban/filter.d/theia-scan.conf
[Definition]
failregex = ^<HOST> .* "(GET|POST|HEAD) /(wp-admin|wp-login\.php|wp-includes|wp-content|xmlrpc\.php|\.git/config|\.env|sftp-config\.json|cgi-bin/).*" (404|444)
ignoreregex =
```

Create a jail that uses it:

```ini
# /etc/fail2ban/jail.d/theia-scan.conf
[theia-scan]
enabled  = true
port     = http,https
filter   = theia-scan
logpath  = /var/log/nginx/access.log
maxretry = 3
findtime = 600
bantime  = 86400
```

This bans an IP for 24 hours (`bantime`) after 3 matching hits (`maxretry`) within a 10-minute
window (`findtime`). Adjust to taste — a stricter `bantime` is reasonable since legitimate clients
never request these paths.

Restart fail2ban and confirm the jail is active:

```bash
sudo systemctl restart fail2ban
sudo fail2ban-client status theia-scan
```

If you configured multi-domain logging via `theia`'s installer (`log_format theia_combined`, see
the main [README](../README.md#installation)), the log line format is unchanged from a standard
combined log plus a trailing `"$host"` field, so the filter regex above still matches — the
`failregex` only needs to match the request line and status code, which come before the appended
host field.
