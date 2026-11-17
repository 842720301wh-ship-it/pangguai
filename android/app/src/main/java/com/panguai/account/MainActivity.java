package com.panguai.account;

import android.app.Activity;
import android.content.Intent;
import android.os.Bundle;
import android.graphics.Color;
import android.net.Uri;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.webkit.WebResourceResponse;
import androidx.activity.ComponentActivity;
import androidx.activity.result.ActivityResultLauncher;
import androidx.activity.result.contract.ActivityResultContracts;
import androidx.core.graphics.Insets;
import androidx.core.view.ViewCompat;
import androidx.core.view.WindowCompat;
import androidx.core.view.WindowInsetsCompat;
import com.journeyapps.barcodescanner.ScanContract;
import com.journeyapps.barcodescanner.ScanOptions;
import android.widget.FrameLayout;
import android.widget.TextView;
import android.widget.Toast;

public final class MainActivity extends ComponentActivity {
    static { System.loadLibrary("panguai"); }
    private static native int startBackend(String directory);
    private WebView webView;
    private PhoneFeatures features;
    private final ActivityResultLauncher<String> imageSaver = registerForActivityResult(
        new ActivityResultContracts.CreateDocument("image/jpeg"), uri -> {
            if (features != null) features.finishSave(uri);
        });
    private final ActivityResultLauncher<ScanOptions> scanner = registerForActivityResult(new ScanContract(), result -> {
        if (features != null) features.finishScan(result.getContents(), null);
    });

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        // Apply system spacing ourselves on every Android version, including edge-to-edge.
        WindowCompat.setDecorFitsSystemWindows(getWindow(), false);
        getWindow().setStatusBarColor(Color.rgb(250,251,252));
        getWindow().setNavigationBarColor(Color.WHITE);
        WindowCompat.getInsetsController(getWindow(), getWindow().getDecorView()).setAppearanceLightStatusBars(true);
        WindowCompat.getInsetsController(getWindow(), getWindow().getDecorView()).setAppearanceLightNavigationBars(true);
        FrameLayout frame = new FrameLayout(this);
        frame.setBackgroundColor(Color.rgb(250,251,252));
        ViewCompat.setOnApplyWindowInsetsListener(frame, (view, insets) -> {
            Insets safeArea = insets.getInsets(WindowInsetsCompat.Type.systemBars()
                | WindowInsetsCompat.Type.displayCutout() | WindowInsetsCompat.Type.ime());
            view.setPadding(safeArea.left, safeArea.top, safeArea.right, safeArea.bottom);
            // The WebView is already inside the safe area; do not apply it a second time in CSS.
            return WindowInsetsCompat.CONSUMED;
        });
        setContentView(frame);
        ViewCompat.requestApplyInsets(frame);
        TextView loading = new TextView(this);
        loading.setText("正在启动本机服务…"); loading.setGravity(android.view.Gravity.CENTER);
        frame.addView(loading,new FrameLayout.LayoutParams(-1,-1));
        new Thread(() -> {
            final int port = startBackend(getFilesDir().getAbsolutePath());
            runOnUiThread(() -> {
                if (isFinishing() || isDestroyed()) return;
                if (port <= 0) { loading.setText("本机服务启动失败，请重新打开应用。"); return; }
                final String origin = "http://127.0.0.1:" + port;
                webView = new WebView(this);
                webView.setBackgroundColor(Color.rgb(250,251,252));
                WebSettings settings = webView.getSettings();
                settings.setJavaScriptEnabled(true);
                settings.setDomStorageEnabled(false);
                settings.setAllowFileAccess(false);
                settings.setAllowContentAccess(false);
                settings.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
                settings.setSaveFormData(false);
                features = new PhoneFeatures(this, webView, origin, () -> scanner.launch(
                    new ScanOptions().setCaptureActivity(ScanActivity.class).setDesiredBarcodeFormats(ScanOptions.QR_CODE)
                        .setBeepEnabled(false).setBarcodeImageEnabled(false).setOrientationLocked(true)
                        .setTimeout(120000).setPrompt("扫描设备二维码")), filename -> imageSaver.launch(filename));
                webView.addJavascriptInterface(features, "PanguaiNative");
                webView.setWebViewClient(new WebViewClient() {
                    private boolean local(Uri target) {
                        return "http".equals(target.getScheme()) && "127.0.0.1".equals(target.getHost()) && target.getPort() == port;
                    }
                    @Override public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                        if (request.isForMainFrame() && request.hasGesture()
                            && "https://github.com/842720301wh-ship-it/pangguai".equals(request.getUrl().toString())) {
                            try {
                                startActivity(new Intent(Intent.ACTION_VIEW, request.getUrl()).addCategory(Intent.CATEGORY_BROWSABLE));
                            } catch (android.content.ActivityNotFoundException e) {
                                Toast.makeText(MainActivity.this, "未找到可打开链接的浏览器", Toast.LENGTH_SHORT).show();
                            }
                            return true;
                        }
                        return !local(request.getUrl());
                    }
                    @Override public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest request) {
                        if (local(request.getUrl())) return null;
                        return new WebResourceResponse("text/plain", "UTF-8", 403, "Blocked", java.util.Collections.emptyMap(), new java.io.ByteArrayInputStream(new byte[0]));
                    }
                });
                frame.removeAllViews(); frame.addView(webView,new FrameLayout.LayoutParams(-1,-1));
                webView.loadUrl(origin + "/");
            });
        }, "panguai-start").start();
    }
    @Override public void onBackPressed() { if(webView!=null && webView.canGoBack())webView.goBack();else super.onBackPressed(); }
    @Override public void onRequestPermissionsResult(int code, String[] permissions, int[] results) {
        super.onRequestPermissionsResult(code, permissions, results);
        if (code == PhoneFeatures.LOCATION_PERMISSION && features != null) features.onLocationPermission();
    }
    @Override protected void onDestroy(){if(features!=null)features.destroy();if(webView!=null){webView.removeJavascriptInterface("PanguaiNative");webView.stopLoading();webView.destroy();webView=null;}super.onDestroy();}
}
