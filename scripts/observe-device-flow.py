"""Observe the user's own device/order state through the local read-only API.

This is state sampling, not an HTTPS packet capture. Only GET requests to the
local backend are sent. It never creates/pays/starts/stops anything, reads App
logs, changes phone settings, or exports authentication credentials.
"""
import argparse
import datetime
import hashlib
import json
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def emit(**data):
    print(json.dumps(data, ensure_ascii=False), flush=True)


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--backend", default="http://127.0.0.1:18080")
    p.add_argument("--account", default="")
    p.add_argument("--duration", type=int, default=900)
    p.add_argument("--interval", type=int, default=10)
    args = p.parse_args()
    parsed = urllib.parse.urlsplit(args.backend)
    if parsed.scheme != "http" or parsed.hostname not in ("127.0.0.1", "localhost", "::1") or parsed.path or parsed.query or parsed.fragment or parsed.username or parsed.password:
        raise ValueError("Backend must be a loopback HTTP origin")
    if not 10 <= args.duration <= 1800 or not 10 <= args.interval <= 60:
        raise ValueError("Duration/interval is outside the bounded observation range")
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

    def get(path):
        with opener.open(args.backend + path, timeout=30) as reply:
            return json.load(reply)

    accounts = get("/api/accounts")["accounts"]
    selected = [a for a in accounts if not args.account or a["id"] == args.account]
    if len(selected) != 1:
        raise ValueError("Select exactly one saved own account with --account")
    prefix = "/api/accounts/" + urllib.parse.quote(selected[0]["id"], safe="")
    devices = get(prefix + "/devices")
    orders = get(prefix + "/orders/recent")
    if devices.get("mode") != "readonly" or orders.get("mode") != "readonly":
        raise ValueError("Read-only observation API is not ready")
    if not devices.get("devices"):
        raise ValueError("No recently used own devices found")
    details = []
    for device in devices["devices"]:
        details.append(get(prefix + "/devices/" + urllib.parse.quote(str(device["goodsId"]), safe="")))
    run = ROOT / "work/pangguai-login/evidence" / ("device-flow-" + uuid.uuid4().hex[:12])
    run.mkdir(parents=True)
    stop_file = run / "stop"
    snapshots = run / "states.jsonl"
    metadata = {"mode": "readonly-state-observation", "https_capture": False, "interval_seconds": args.interval, "max_duration_seconds": args.duration, "started_at": datetime.datetime.now(datetime.timezone.utc).isoformat(), "device_count": len(devices["devices"]), "baseline_order_count": len(orders.get("orders", [])), "mutations_sent": False, "credentials_saved": False}
    (run / "metadata.json").write_text(json.dumps(metadata, indent=2), encoding="utf-8")
    (run / "device-details.json").write_text(json.dumps(details, ensure_ascii=False, indent=2), encoding="utf-8")
    previous = ""
    changes = 0
    deadline = time.monotonic() + args.duration

    def record(device_reply, order_reply):
        nonlocal previous, changes
        state = {"devices": device_reply["devices"], "orders": order_reply["orders"]}
        canonical = json.dumps(state, ensure_ascii=False, sort_keys=True)
        if canonical == previous:
            return
        previous = canonical
        changes += 1
        with snapshots.open("a", encoding="utf-8") as out:
            out.write(json.dumps({"observed_at": datetime.datetime.now(datetime.timezone.utc).isoformat(), **state}, ensure_ascii=False) + "\n")
        summarize = lambda text: hashlib.sha256(str(text).encode()).hexdigest()[:8]
        emit(status="state_changed", sample=changes, devices=[{"ref": summarize(d["goodsId"]), "work_status": d.get("workStatus")} for d in state["devices"]], orders=[{"ref": summarize(o.get("id", o.get("orderNo", ""))), "state": o.get("orderStatus")} for o in state["orders"][:5]])

    record(devices, orders)
    emit(status="ready", **metadata, output=str(run.relative_to(ROOT)), stop_file=str(stop_file.relative_to(ROOT)))
    try:
        while time.monotonic() < deadline and not stop_file.exists():
            wait_until = min(time.monotonic() + args.interval, deadline)
            while time.monotonic() < wait_until and not stop_file.exists():
                time.sleep(min(1, max(0, wait_until - time.monotonic())))
            if stop_file.exists() or time.monotonic() >= deadline:
                break
            devices = get(prefix + "/devices")
            orders = get(prefix + "/orders/recent")
            record(devices, orders)
    except KeyboardInterrupt:
        pass
    finally:
        metadata.update({"finished_at": datetime.datetime.now(datetime.timezone.utc).isoformat(), "changed_samples": changes})
        (run / "metadata.json").write_text(json.dumps(metadata, indent=2), encoding="utf-8")
        emit(status="stopped", changed_samples=changes, mutations_sent=False, output=str(run.relative_to(ROOT)))


if __name__ == "__main__":
    try:
        main()
    except urllib.error.HTTPError as e:
        try:
            reply = json.loads(e.read(4096))
        except Exception:
            reply = {}
        emit(status="observation_not_ready", http_status=e.code, code=reply.get("code"), upstream_path=reply.get("upstream_path"))
        sys.exit(1)
    except Exception as e:
        emit(status="observation_failed", error_type=type(e).__name__)
        sys.exit(1)
