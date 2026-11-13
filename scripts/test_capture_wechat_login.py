"""Offline parser/privacy regression checks. No ADB or real credentials."""
import importlib.util
import io
import unittest
import urllib.error
from pathlib import Path
from unittest.mock import Mock, patch

spec = importlib.util.spec_from_file_location("wechat_capture", Path(__file__).with_name("capture-wechat-login.py"))
capture = importlib.util.module_from_spec(spec)
spec.loader.exec_module(capture)


class CaptureTests(unittest.TestCase):
    def test_only_marked_network_helper_results_are_decoded(self):
        self.assertIsNone(capture.decode_log_payload('{"access_token":"fixture","openid":"fixture-open"}'))
        self.assertEqual(capture.decode_log_payload('httpResult={"access_token":"fixture","openid":"fixture-open"}')["openid"], "fixture-open")

    def test_multiline_grant_is_not_parsed_until_complete(self):
        prefix = 'httpResult={\n"access_token":"fixture",\n'
        self.assertIsNone(capture.decode_log_payload(prefix))
        self.assertEqual(capture.decode_log_payload(prefix + '"openid":"fixture-open"\n}')["access_token"], "fixture")

    def test_nonobject_and_broken_payloads_do_not_become_credentials(self):
        self.assertEqual(capture.decode_log_payload('httpResult=["fixture"]'), {})
        self.assertIsNone(capture.decode_log_payload('httpResult=not json'))

    def test_error_output_cannot_contain_upstream_credentials(self):
        failure = urllib.error.HTTPError("http://127.0.0.1:18080/api/login/wechat/confirm", 502, "fixture", {}, io.BytesIO(b'{"code":"fixture_error","upstream_path":"/user/info","token":"fixture-sensitive","message":"fixture-sensitive"}'))
        opener = Mock()
        opener.open.side_effect = failure
        with patch.object(capture.urllib.request, "build_opener", return_value=opener):
            status, reply = capture.local_request("http://127.0.0.1:18080", "/api/login/wechat/confirm", {"access_token": "fixture-sensitive"})
        self.assertEqual(status, 502)
        self.assertEqual(reply, {"code": "fixture_error", "upstream_path": "/user/info"})
        self.assertNotIn("fixture-sensitive", str(reply))


if __name__ == "__main__":
    unittest.main()
