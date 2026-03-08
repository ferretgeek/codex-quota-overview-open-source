package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"codex-overview-backend/internal/app"
)

func main() {
	workingDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("resolve working directory failed: %v", err)
	}
	appDir := filepath.Dir(workingDir)
	workspaceRoot := filepath.Dir(appDir)
	staticDir := filepath.Join(appDir, "web", "dist")

	addrFlag := flag.String("addr", "127.0.0.1:8787", "http listen address")
	workspaceFlag := flag.String("workspace-root", workspaceRoot, "workspace root containing auth directories")
	staticFlag := flag.String("static-dir", staticDir, "frontend dist directory")
	openBrowserFlag := flag.Bool("open-browser", true, "open browser after server starts")
	cacheTTLFlag := flag.Duration("cache-ttl", 20*time.Second, "snapshot cache ttl")
	flag.Parse()

	server := app.NewServer(app.ServerConfig{
		AppRoot:       appDir,
		WorkspaceRoot: strings.TrimSpace(*workspaceFlag),
		StaticDir:     strings.TrimSpace(*staticFlag),
		CacheTTL:      *cacheTTLFlag,
		AppName:       "Codex普号额度概览",
		DefaultPrice:  7.5,
	})

	address := strings.TrimSpace(*addrFlag)
	if address == "" {
		address = "127.0.0.1:8787"
	}

	httpServer := &http.Server{
		Addr:              address,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	url := fmt.Sprintf("http://%s", address)
	log.Printf("%s 启动中，逻辑 CPU=%d，工作目录=%s", server.Config().AppName, runtime.NumCPU(), server.Config().WorkspaceRoot)
	log.Printf("打开地址：%s", url)

	if *openBrowserFlag {
		go func() {
			time.Sleep(900 * time.Millisecond)
			_ = app.OpenBrowser(url)
		}()
	}

	if err = httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server exited: %v", err)
	}
}
