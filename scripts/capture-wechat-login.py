"""Observe one user-initiated official-App OAuth grant over authorized ADB.

Only NetworkUtil log messages from com.qiekj.user's current PID are read.
Raw logs and OAuth credentials never go to stdout or disk. The normal local
backend verifies the grant, reads back identity and encrypts the account.
No proxy, VPN, certificate, APK, log buffer or phone setting is changed.
"""
import argparse
import datetime
import json
import queue
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
ADB = "C:/Users/84272/AppData/Local/Android/Sdk/platform-tools/adb.exe"


def emit(**value):
    print(json.dumps(value, ensure_ascii=False), flush=True)


def local_request(origin, path, body=None):
    headers = {"Content-Type": "application/json", "X-Panguai-Client": "web"}
    raw = None if body is None else json.dumps(body).encode("utf-8")
    request = urllib.request.Request(origin + path, data=raw, headers=headers)
    # Local-only control traffic must not use the host's normal Internet proxy.
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    try:
        with opener.open(request, timeout=30) as response:
            return response.status, json.load(response)
    except urllib.error.HTTPError as error:
        try:
            reply = json.loads(error.read(8192))
        except (ValueError, OSError):
            reply = {}
        return error.code, {key: reply[key] for key in ("code", "upstream_path") if key in reply}


def decode_log_payload(buffer):
    marker = "httpResult="
    position = buffer.find(marker)
    if position < 0:
        return None
    try:
        result, _ = json.JSONDecoder().raw_decode(buffer[position + len(marker):].lstrip())
    except ValueError:
        return None
    return result if isinstance(result, dict) else {}


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--serial", default="3B666401BLM00000")
    parser.add_argument("--backend", default="http://127.0.0.1:18080")
    parser.add_argument("--timeout", type=int, default=240)
    arguments = parser.parse_args()
    origin = urllib.parse.urlsplit(arguments.backend)
    if origin.scheme != "http" or origin.hostname not in ("127.0.0.1", "localhost", "::1") or origin.username or origin.password or origin.path or origin.query or origin.fragment:
        raise ValueError("Backend must be a loopback HTTP origin")
    if not 10 <= arguments.timeout <= 600:
        raise ValueError("Timeout must be between 10 and 600 seconds")
    status, ready = local_request(arguments.backend, "/api/status")
    if status != 200 or not ready.get("protocol_ready") or "wechat_credential" not in ready.get("methods", []):
        emit(status="backend_unavailable")
        return 2
    adb = [ADB, "-s", arguments.serial]
    result = subprocess.run(adb + ["shell", "pidof", "com.qiekj.user"], capture_output=True, timeout=8)
    pids = result.stdout.decode("ascii", "ignore").strip().split()
    if not pids or not pids[0].isdigit():
        emit(status="official_app_not_running", message="请先打开官方胖乖生活，再运行采集脚本")
        return 2
    # Use the phone's clock so only fresh results from this operation are read.
    clock = subprocess.run(adb + ["shell", "date '+%m-%d %H:%M:%S.000'"], capture_output=True, timeout=8).stdout.decode("ascii", "ignore").strip()
    try:
        datetime.datetime.strptime(clock, "%m-%d %H:%M:%S.%f")
    except ValueError:
        emit(status="phone_clock_unavailable")
        return 2
    # Both interpolated fields were strictly validated above; quote the remote
    # timestamp so adb's shell cannot split it into two arguments.
    command = "logcat -v raw -T '" + clock + "' --pid=" + pids[0] + " NetworkUtil:E '*:S'"
    process = subprocess.Popen(adb + ["shell", command], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, encoding="utf-8", errors="replace", bufsize=1, creationflags=subprocess.CREATE_NO_WINDOW)
    lines = queue.Queue(maxsize=1000)

    def read():
        for line in process.stdout:
            try:
                lines.put(line, timeout=1)
            except queue.Full:
                # Stop instead of collecting unbounded sensitive material.
                process.terminate()
                break
        lines.put(None)

    threading.Thread(target=read, daemon=True).start()
    emit(status="ready", package="com.qiekj.user", scope="NetworkUtil only; new messages", timeout_seconds=arguments.timeout, raw_logs_saved=False, sms_sent=False)
    deadline = time.monotonic() + arguments.timeout
    pending = ""
    try:
        while time.monotonic() < deadline:
            try:
                line = lines.get(timeout=1)
            except queue.Empty:
                if process.poll() is not None:
                    emit(status="adb_log_stream_closed")
                    return 2
                continue
            if line is None:
                emit(status="adb_log_stream_closed")
                return 2
            if "httpResult=" in line:
                pending = line[line.index("httpResult="):]
            elif pending:
                pending += line
            if len(pending) > 65536:
                pending = ""
                continue
            reply = decode_log_payload(pending)
            if reply is None:
                continue
            pending = ""
            access = reply.get("access_token")
            open_id = reply.get("openid")
            if not isinstance(access, str) or not isinstance(open_id, str) or not access or not open_id:
                # Public progress only; no user identity or upstream text.
                if isinstance(reply.get("errcode"), int):
                    emit(status="official_wechat_error", wechat_error_code=reply["errcode"])
                continue
            emit(status="oauth_grant_observed", credential_saved=False, action="verify_with_local_backend")
            response_status, response = local_request(arguments.backend, "/api/login/wechat/confirm", {"access_token": access, "openid": open_id})
            access = open_id = ""
            reply.clear()
            if response_status != 200:
                emit(status="backend_verification_failed", http_status=response_status, **response)
                return 1
            account = response.get("account", {})
            account_id = account.get("id")
            if response.get("status") != "confirmed" or not account_id:
                emit(status="backend_result_incomplete")
                return 1
            check_status, check = local_request(arguments.backend, "/api/accounts/" + urllib.parse.quote(account_id, safe="") + "/check", {})
            emit(status="confirmed" if check_status == 200 and check.get("status") == "valid" else "saved_but_recheck_failed", account_id=account_id, phone=account.get("phone", ""), readback_valid=check_status == 200 and check.get("status") == "valid", sms_sent=False, raw_logs_saved=False)
            return 0 if check_status == 200 else 1
        emit(status="timeout", message="本次等待未收到新的微信授权结果，未保存凭据，未发送短信")
        return 2
    finally:
        process.terminate()
        try:
            process.wait(timeout=3)
        except subprocess.TimeoutExpired:
            process.kill()


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        emit(status="cancelled")
        sys.exit(130)
    except Exception as error:
        # Exceptions may contain request details. Output type only.
        emit(status="capture_failed", error_type=type(error).__name__)
        sys.exit(2)
