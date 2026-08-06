# Shared proxy headers and timeouts.
proxy_http_version 1.1;
proxy_set_header Connection "";

proxy_set_header Host              $host;
proxy_set_header X-Real-IP         $remote_addr;
proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto $scheme;
# Passed through so one identifier follows a request across NGINX and the API.
proxy_set_header X-Request-ID      $request_id;

proxy_connect_timeout 5s;
proxy_send_timeout    60s;
proxy_read_timeout    60s;

# Retry the next upstream only on failures that are safe to repeat. Notably
# absent: non_idempotent — replaying a POST that already reached the API could
# create a second booking or a second payment.
proxy_next_upstream error timeout http_502 http_503 http_504;
proxy_next_upstream_tries 2;
