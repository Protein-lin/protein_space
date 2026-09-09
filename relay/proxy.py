#!/usr/bin/env python3
"""Minimal streaming HTTP reverse proxy for an internal relay host.

Environment:
  RELAY_BIND=127.0.0.1
  RELAY_PORT=8443
  UPSTREAMS=https://10.0.0.21,https://10.0.0.22
  UPSTREAM_HOST=ai-api.ort.sealaly.com  # optional Host/SNI name
"""
import itertools
import os
import ssl
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def csv(name, default=""):
    return [x.strip().rstrip("/") for x in os.getenv(name, default).split(",") if x.strip()]


UPSTREAMS = csv("UPSTREAMS")
if not UPSTREAMS:
    raise SystemExit("UPSTREAMS is required, e.g. https://10.0.0.21,https://10.0.0.22")
UPSTREAM_HOST = os.getenv("UPSTREAM_HOST", "")
counter = itertools.cycle(UPSTREAMS)


class RelayHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def do_GET(self):
        if self.path == "/health":
            body = b'{"status":"ok","service":"relay"}\n'
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        self.forward()

    def do_POST(self):
        self.forward()

    def forward(self):
        upstream = next(counter) + self.path
        length = int(self.headers.get("Content-Length", "0"))
        payload = self.rfile.read(length) if length else None
        headers = {k: v for k, v in self.headers.items() if k.lower() not in {"host", "content-length", "connection"}}
        if UPSTREAM_HOST:
            headers["Host"] = UPSTREAM_HOST
        request = urllib.request.Request(upstream, data=payload, headers=headers, method=self.command)
        context = ssl.create_default_context()
        try:
            with urllib.request.urlopen(request, context=context, timeout=600) as response:
                self.send_response(response.status)
                for key, value in response.headers.items():
                    if key.lower() not in {"connection", "transfer-encoding", "content-length"}:
                        self.send_header(key, value)
                self.send_header("Cache-Control", "no-cache")
                self.send_header("X-Accel-Buffering", "no")
                self.send_header("Connection", "close")
                self.end_headers()
                while True:
                    chunk = response.read(8192)
                    if not chunk:
                        break
                    self.wfile.write(chunk)
                    self.wfile.flush()
        except urllib.error.HTTPError as exc:
            body = exc.read(8192)
            self.send_response(exc.code)
            self.send_header("Content-Type", exc.headers.get("Content-Type", "text/plain"))
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        except Exception as exc:
            body = ("relay error: " + str(exc) + "\n").encode()
            self.send_response(502)
            self.send_header("Content-Type", "text/plain; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    def log_message(self, fmt, *args):
        print("[relay] " + (fmt % args), flush=True)


server = ThreadingHTTPServer((os.getenv("RELAY_BIND", "127.0.0.1"), int(os.getenv("RELAY_PORT", "8443"))), RelayHandler)
print(f"relay listening on {server.server_address}, upstreams={UPSTREAMS}", flush=True)
server.serve_forever()
