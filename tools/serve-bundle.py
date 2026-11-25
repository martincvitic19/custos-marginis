#!/usr/bin/env python3
# PC side of a streamed RAUC install (/data is too small for a second bundle):
#   python3 tools/serve-bundle.py <bundle.raucb> 38471 &
#   ssh -R 127.0.0.1:38471:127.0.0.1:38471 root@192.168.20.1 rauc install http://127.0.0.1:38471/bundle.raucb
# One file, loopback only, HTTP range requests (RAUC streaming needs them).
import http.server, os, re, sys
F, PORT = sys.argv[1], int(sys.argv[2])
SIZE = os.path.getsize(F)
class H(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    def log_message(self, *a): pass
    def do_HEAD(self): self.serve(False)
    def do_GET(self): self.serve(True)
    def serve(self, body):
        if self.path != "/bundle.raucb":
            self.send_response(404); self.send_header("Content-Length","0"); self.end_headers(); return
        a, b, code = 0, SIZE-1, 200
        m = re.fullmatch(r"bytes=(\d+)-(\d*)", self.headers.get("Range",""))
        if m:
            a = int(m.group(1)); b = min(int(m.group(2) or SIZE-1), SIZE-1); code = 206
            if a > b:
                self.send_response(416); self.send_header("Content-Length","0"); self.end_headers(); return
        self.send_response(code)
        self.send_header("Accept-Ranges","bytes")
        self.send_header("Content-Length",str(b-a+1))
        if code == 206: self.send_header("Content-Range",f"bytes {a}-{b}/{SIZE}")
        self.end_headers()
        if body:
            with open(F,"rb") as f:
                f.seek(a); n = b-a+1
                while n:
                    d = f.read(min(n,262144))
                    if not d: break
                    self.wfile.write(d); n -= len(d)
http.server.ThreadingHTTPServer(("127.0.0.1", PORT), H).serve_forever()
