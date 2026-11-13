package server

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"panguai/internal/accounts"
	"panguai/internal/platform"
	"sync"
	"time"
)

type session struct {
	phone        string
	expires      time.Time
	attempts     int
	busy         bool
	confirmed    bool
	profile      accounts.Profile
	pendingToken string
	lastError    map[string]string
}

type Gateway interface {
	SendSMS(context.Context, string) error
	Login(context.Context, string, string) (platform.LoginResult, error)
	Check(context.Context, string) (platform.LoginResult, error)
}
type Server struct {
	store      *accounts.Store
	platform   Gateway
	mu         sync.Mutex
	sessions   map[string]*session
	cooldown   map[string]time.Time
	inflight   int
	usage      map[string]*usageIntent
	usageLocks sync.Map
	liveCache  sync.Map
}

func New(store *accounts.Store, client Gateway, webDir string) http.Handler {
	var web fs.FS
	if webDir != "" {
		web = os.DirFS(webDir)
	}
	return NewWithFS(store, client, web)
}

func NewWithFS(store *accounts.Store, client Gateway, web fs.FS) http.Handler {
	s := &Server{store: store, platform: client, sessions: map[string]*session{}, cooldown: map[string]time.Time{}, usage: map[string]*usageIntent{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/accounts", s.list)
	mux.HandleFunc("GET /api/accounts/{id}/devices", s.recentDevices)
	mux.HandleFunc("POST /api/accounts/{id}/devices/scan", s.scanDevice)
	mux.HandleFunc("GET /api/accounts/{id}/tickets", s.tickets)
	mux.HandleFunc("GET /api/accounts/{id}/devices/{device}", s.deviceDetails)
	mux.HandleFunc("GET /api/accounts/{id}/orders/recent", s.recentOrders)
	mux.HandleFunc("GET /api/accounts/{id}/orders/{order}/start-status", s.usageStartStatus)
	mux.HandleFunc("GET /api/accounts/{id}/devices/{device}/usage/options", s.usageOptions)
	mux.HandleFunc("POST /api/accounts/{id}/devices/{device}/usage/prepare", s.prepareUsage)
	mux.HandleFunc("POST /api/accounts/{id}/devices/{device}/usage/start", s.startUsage)
	mux.HandleFunc("GET /api/accounts/{id}/devices/{device}/usage/state", s.usageState)
	mux.HandleFunc("POST /api/accounts/{id}/devices/{device}/usage/stop", s.stopUsage)
	mux.HandleFunc("POST /api/login/sms/send", s.sendSMS)
	mux.HandleFunc("POST /api/login/sms/confirm", s.login)
	mux.HandleFunc("POST /api/login/wechat/confirm", s.wechatLogin)
	mux.HandleFunc("GET /api/login/sessions/{id}", s.sessionStatus)
	mux.HandleFunc("POST /api/accounts/{id}/check", s.check)
	mux.HandleFunc("DELETE /api/accounts/{id}", s.remove)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeError(w, 404, "not_found", "接口不存在") })
	if web != nil {
		mux.Handle("/", http.FileServer(http.FS(web)))
	}
	return localOnly(mux)
}
