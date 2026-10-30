"""Exercise an already running, disposable Compose stack (no production data)."""
import json
import os
import subprocess
import time
import urllib.error
import urllib.request

base = "http://127.0.0.1:" + os.environ.get("APP_PORT", "8080")


def compose(*args):
    return subprocess.check_output(["docker", "compose", *args], text=True, timeout=90).strip()


def request(path, method="GET", data=None):
    body = None if data is None else json.dumps(data).encode()
    req = urllib.request.Request(base + path, data=body, method=method,
                                 headers={"Content-Type": "application/json"})
    try:
        response = urllib.request.urlopen(req, timeout=5)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        return response.code, response.read()


def wait_status(path, expected):
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        try:
            if request(path)[0] == expected:
                return
        except (OSError, TimeoutError):
            pass
        time.sleep(0.5)
    raise AssertionError(f"{path} did not return {expected}")


wait_status("/readyz", 200)
assert request("/")[0] == 200
assert request("/static/js/app.js")[0] == 200
assert request("/livez")[0] == 200
assert request("/health")[0] == 200
assert compose("exec", "-T", "app", "id", "-u") == "10001"
for service in ("postgres", "redis"):
    container = compose("ps", "-q", service)
    bindings = json.loads(subprocess.check_output(
        ["docker", "inspect", "--format", "{{json .HostConfig.PortBindings}}", container], text=True))
    assert not bindings, f"{service} must not publish host ports"

status, payload = request("/api/boards", "POST")
assert status == 201, payload
board = json.loads(payload)
path = "/api/boards/" + board["id"]
update = {"base_revision": 0, "code_html": "<h1>Compose works</h1>", "code_css": "h1 { color: teal; }"}
status, payload = request(path, "PUT", update)
assert status == 200 and json.loads(payload)["revision"] == 1, payload
assert request(path, "PUT", update)[0] == 409

for dependency in ("redis", "postgres"):
    compose("stop", dependency)
    wait_status("/readyz", 503)
    assert request("/livez")[0] == 200
    compose("start", dependency)
    wait_status("/readyz", 200)

compose("restart", "app")
wait_status("/readyz", 200)
status, payload = request(path)
saved = json.loads(payload)
assert status == 200 and saved["revision"] == 1 and saved["code_html"] == update["code_html"], payload
print("Compose smoke passed: startup, static files, non-root, isolated databases, CRUD/CAS, outages/recovery, persistence")
