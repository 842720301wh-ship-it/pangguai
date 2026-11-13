package com.panguai.account;

import android.Manifest;
import android.app.Activity;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.pm.PackageManager;
import android.location.Location;
import android.location.LocationListener;
import android.location.LocationManager;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.os.SystemClock;
import android.net.Uri;
import android.webkit.JavascriptInterface;
import android.webkit.WebView;
import org.json.JSONObject;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.util.function.Consumer;

/** Foreground, one-shot capabilities for the embedded loopback UI only. */
final class PhoneFeatures {
    static final int LOCATION_PERMISSION = 201;
    private final Activity activity;
    private final WebView view;
    private final String origin;
    private final Runnable launchScanner;
    private final Consumer<String> launchSave;
    private final Handler handler = new Handler(Looper.getMainLooper());
    private final LocationManager locations;
    private String locationID, scanID;
    private String saveID, saveKey;
    private boolean destroyed;
    private final Runnable timeout = () -> finishLocation(null, "定位超时，请到信号较好的位置重试");
    private final LocationListener listener = new LocationListener() {
        @Override public void onLocationChanged(Location location) {
            // A cached/stale or very coarse fix must never silently start a nearby device.
            if (fresh(location)) finishLocation(location, null);
        }
        @Override public void onProviderEnabled(String provider) { }
        @Override public void onProviderDisabled(String provider) { }
        @Override public void onStatusChanged(String provider, int status, Bundle extras) { }
    };

    PhoneFeatures(Activity activity, WebView view, String origin, Runnable launchScanner, Consumer<String> launchSave) {
        this.activity = activity; this.view = view; this.origin = origin;
        this.launchScanner = launchScanner;
        this.launchSave = launchSave;
        locations = (LocationManager) activity.getSystemService(Activity.LOCATION_SERVICE);
    }
    private boolean trusted(String id) {
        String url = view.getUrl();
        return !destroyed && !activity.isFinishing() && id != null && id.matches("[a-zA-Z0-9-]{1,80}")
            && url != null && (url.equals(origin + "/") || url.startsWith(origin + "/"));
    }
    @JavascriptInterface public void requestLocation(String id) {
        handler.post(() -> {
            if (!trusted(id)) return;
            if (locationID != null) { reply(id, null, "定位正在进行，请稍候"); return; }
            locationID = id;
            if (activity.checkSelfPermission(Manifest.permission.ACCESS_FINE_LOCATION) != PackageManager.PERMISSION_GRANTED) {
                activity.requestPermissions(new String[]{Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION}, LOCATION_PERMISSION);
            } else locate();
        });
    }
    void onLocationPermission() {
        if (locationID == null) return;
        if (activity.checkSelfPermission(Manifest.permission.ACCESS_FINE_LOCATION) != PackageManager.PERMISSION_GRANTED) {
            finishLocation(null, "请在系统权限中允许精确定位后重试");
        } else locate();
    }
    private boolean fresh(Location p) {
        long age = SystemClock.elapsedRealtimeNanos() - p.getElapsedRealtimeNanos();
        return age >= 0 && age <= 60_000_000_000L && p.hasAccuracy() && p.getAccuracy() <= 200
            && Double.isFinite(p.getLatitude()) && Double.isFinite(p.getLongitude())
            && (p.getLatitude() != 0 || p.getLongitude() != 0);
    }
    private void locate() {
        boolean enabled = false;
        try {
            for (String provider : new String[]{LocationManager.GPS_PROVIDER, LocationManager.NETWORK_PROVIDER}) {
                if (!locations.isProviderEnabled(provider)) continue;
                enabled = true;
                Location last = locations.getLastKnownLocation(provider);
                if (last != null && fresh(last)) { finishLocation(last, null); return; }
                locations.requestLocationUpdates(provider, 1000L, 0f, listener, Looper.getMainLooper());
            }
            if (!enabled) { finishLocation(null, "请先打开手机系统定位服务"); return; }
            handler.postDelayed(timeout, 25000);
        } catch (SecurityException e) { finishLocation(null, "定位权限不可用，请重新允许精确定位"); }
        catch (RuntimeException e) { finishLocation(null, "暂时无法取得系统位置，请稍后重试"); }
    }
    private void finishLocation(Location p, String error) {
        String id = locationID;
        locationID = null;
        handler.removeCallbacks(timeout);
        try { locations.removeUpdates(listener); } catch (RuntimeException ignored) { }
        if (id == null) return;
        JSONObject result = null;
        if (p != null) {
            result = new JSONObject();
            try {
                result.put("latitude", p.getLatitude()).put("longitude", p.getLongitude())
                    .put("accuracy", p.getAccuracy()).put("timestamp", p.getTime())
                    .put("coordinate_system", "wgs84");
            } catch (org.json.JSONException e) { error = "位置数据无效"; result = null; }
        }
        reply(id, result, error);
    }
    @JavascriptInterface public void scanCode(String id) {
        handler.post(() -> {
            if (!trusted(id)) return;
            if (scanID != null || locationID != null) { reply(id, null, "另一个操作正在进行，请稍候"); return; }
            scanID = id;
            try { launchScanner.run(); }
            catch (RuntimeException e) { finishScan(null, "无法打开相机，请检查相机权限"); }
        });
    }
    void finishScan(String contents, String error) {
        String id = scanID; scanID = null;
        if (id == null) return;
        JSONObject result = null;
        if (contents != null && !contents.trim().isEmpty() && contents.length() <= 2048) {
            result = new JSONObject();
            try { result.put("text", contents); } catch (org.json.JSONException ignored) { }
        } else if (error == null) error = contents == null ? "scan_cancelled" : "二维码内容无效";
        reply(id, result, error);
    }
    @JavascriptInterface public void copyGroupNumber(String id) {
        handler.post(() -> {
            if (!trusted(id)) return;
            try {
                ClipboardManager clipboard = (ClipboardManager) activity.getSystemService(Activity.CLIPBOARD_SERVICE);
                clipboard.setPrimaryClip(ClipData.newPlainText("水到渠成 QQ 群号", "345353411"));
                reply(id, new JSONObject(), null);
            } catch (RuntimeException e) { reply(id, null, "复制失败，请手动复制群号：345353411"); }
        });
    }
    @JavascriptInterface public void saveImage(String id, String key) {
        handler.post(() -> {
            if (!trusted(id)) return;
            if (!"sponsor".equals(key) && !"contact".equals(key)) { reply(id, null, "图片不存在"); return; }
            if (saveID != null) { reply(id, null, "图片正在保存，请稍候"); return; }
            saveID = id; saveKey = key;
            try { launchSave.accept("sponsor".equals(key) ? "水到渠成-赞赏码.jpg" : "水到渠成-QQ群.jpg"); }
            catch (RuntimeException e) { saveID = null; saveKey = null; reply(id, null, "无法打开系统保存界面，请稍后重试"); }
        });
    }
    void finishSave(Uri uri) {
        final String id = saveID, key = saveKey;
        if (id == null) return;
        if (uri == null) { saveID = null; saveKey = null; reply(id, null, "save_cancelled"); return; }
        // Only the two bundled images can be exported, to a user-selected document URI.
        new Thread(() -> {
            String error = null;
            HttpURLConnection connection = null;
            try {
                if (!"content".equals(uri.getScheme())) throw new IOException("Invalid save URI");
                connection = (HttpURLConnection) new URL(origin + "/about/" + key + ".jpg").openConnection();
                connection.setInstanceFollowRedirects(false);
                connection.setConnectTimeout(10000); connection.setReadTimeout(15000);
                if (connection.getResponseCode() != 200) throw new IOException("Image unavailable");
                ByteArrayOutputStream image = new ByteArrayOutputStream();
                try (InputStream input = connection.getInputStream()) {
                    byte[] chunk = new byte[8192]; int length;
                    while ((length = input.read(chunk)) != -1) {
                        if (image.size() + length > 5 * 1024 * 1024) throw new IOException("Image too large");
                        image.write(chunk, 0, length);
                    }
                }
                byte[] bytes = image.toByteArray();
                if (bytes.length < 3 || bytes[0] != (byte) 0xff || bytes[1] != (byte) 0xd8) throw new IOException("Invalid image");
                try (OutputStream output = activity.getContentResolver().openOutputStream(uri, "w")) {
                    if (output == null) throw new IOException("Cannot save image");
                    output.write(bytes);
                }
            } catch (IOException | RuntimeException e) { error = "图片保存失败，请重新选择保存位置"; }
            finally { if (connection != null) connection.disconnect(); }
            final String resultError = error;
            handler.post(() -> {
                if (!id.equals(saveID)) return;
                saveID = null; saveKey = null;
                reply(id, resultError == null ? new JSONObject() : null, resultError);
            });
        }, "panguai-save-image").start();
    }
    private void reply(String id, JSONObject data, String error) {
        if (!trusted(id)) return;
        JSONObject message = new JSONObject();
        try {
            message.put("id", id).put("data", data == null ? JSONObject.NULL : data)
                .put("error", error == null ? JSONObject.NULL : error);
            view.evaluateJavascript("window.dispatchEvent(new CustomEvent('panguai-native-result',{detail:" + message + "}))", null);
        } catch (org.json.JSONException ignored) { }
    }
    void destroy() {
        destroyed = true; locationID = null; scanID = null; saveID = null; saveKey = null;
        handler.removeCallbacksAndMessages(null);
        try { locations.removeUpdates(listener); } catch (RuntimeException ignored) { }
    }
}
