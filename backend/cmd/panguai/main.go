package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"panguai/internal/accounts"
	"panguai/internal/platform"
	"panguai/internal/server"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18080", "本机监听地址")
	dataDir := flag.String("data", "../data", "本机数据目录")
	webDir := flag.String("web", "", "可选前端目录，默认仅提供后端接口")
	flag.Parse()
	host, _, err := net.SplitHostPort(*addr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		log.Fatal("服务必须绑定本机回环 IP")
	}
	store, err := accounts.Open(*dataDir)
	if err != nil {
		log.Fatal("本机账号存储无法打开：", err)
	}
	client, err := platform.Load(filepath.Join(*dataDir, "platform.json"))
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Fatal("本机平台配置无效")
		}
		log.Print("平台配置尚未准备，真实登录暂不可用")
	}
	var gateway server.Gateway
	if client != nil {
		gateway = client
	}
	srv := &http.Server{Addr: *addr, Handler: server.New(store, gateway, *webDir), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Printf("胖乖生活本地 API：http://%s/api/status", *addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
