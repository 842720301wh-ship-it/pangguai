//go:build android && cgo

package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"panguai/internal/accounts"
	"panguai/internal/platform"
	"panguai/internal/server"
)

//go:embed all:dist runtime-config.json
var assets embed.FS
var once sync.Once
var port C.int

//export StartPanguai
func StartPanguai(directory *C.char) C.int {
	once.Do(func() {
		root := C.GoString(directory)
		fail := func() {
			_ = os.WriteFile(filepath.Join(root, "startup-error.txt"), []byte("Local service startup failed"), 0600)
		}
		temp := filepath.Join(root, "tmp")
		if os.MkdirAll(temp, 0700) != nil || os.Setenv("TMPDIR", temp) != nil {
			fail()
			return
		}
		store, err := accounts.Open(root)
		if err != nil {
			fail()
			return
		}
		raw, err := assets.ReadFile("runtime-config.json")
		if err != nil {
			fail()
			return
		}
		var config platform.Config
		if json.Unmarshal(raw, &config) != nil {
			fail()
			return
		}
		idPath := filepath.Join(root, "client-id")
		id, err := os.ReadFile(idPath)
		if os.IsNotExist(err) {
			b := make([]byte, 16)
			if _, err = rand.Read(b); err != nil {
				fail()
				return
			}
			id = []byte(hex.EncodeToString(b))
			if os.WriteFile(idPath, id, 0600) != nil {
				fail()
				return
			}
		} else if err != nil {
			fail()
			return
		}
		config.DeviceID = string(id)
		config.ProxyURL = ""
		client, err := platform.New(config)
		if err != nil {
			fail()
			return
		}
		web, err := fs.Sub(assets, "dist")
		if err != nil {
			fail()
			return
		}
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			fail()
			return
		}
		port = C.int(listener.Addr().(*net.TCPAddr).Port)
		service := &http.Server{Handler: server.NewWithFS(store, client, web), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
		go func() { _ = service.Serve(listener) }()
	})
	return port
}

func main() {}
