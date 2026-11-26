package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"meshdelivery/internal/core"
	"meshdelivery/internal/httpapi"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	if e := run(); e != nil {
		slog.Error("mesh stopped", "error", e)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: mesh migrate|import|api|worker|token")
	}
	mode := os.Args[1]
	f := flag.NewFlagSet(mode, flag.ContinueOnError)
	addr := f.String("addr", "127.0.0.1:8081", "HTTP listen address")
	contracts := f.String("contracts", "../docs/openapi", "OpenAPI directory")
	dataset := f.String("dataset", "testdata/demo", "dataset directory")
	all := f.Bool("all", false, "import all ready datasets under --dataset")
	subject := f.String("subject", "demo-user", "token subject")
	roles := f.String("roles", "dispatcher,demo_admin", "comma separated roles")
	points := f.String("points", "", "comma separated pickup point scope")
	sellers := f.String("sellers", "", "comma separated seller scope")
	service := f.Bool("service", false, "issue internal service token")
	if e := f.Parse(os.Args[2:]); e != nil {
		return e
	}
	userKey := []byte(os.Getenv("MESH_USER_JWT_SECRET"))
	serviceKey := []byte(os.Getenv("MESH_SERVICE_JWT_SECRET"))
	issuer := os.Getenv("MESH_JWT_ISSUER")
	if issuer == "" {
		issuer = "mesh-local"
	}
	if mode == "token" {
		key := userKey
		aud := "mesh-user"
		if *service {
			key = serviceKey
			aud = "mesh-internal"
		}
		if len(key) < 32 {
			return fmt.Errorf("JWT secret requires at least 32 bytes")
		}
		fmt.Println(httpapi.Token(key, core.M{"iss": issuer, "sub": *subject, "aud": aud, "exp": time.Now().Add(time.Hour).Unix(), "roles": split(*roles), "pvzIds": split(*points), "sellerIds": split(*sellers)}))
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	dsn := os.Getenv("MESH_DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("MESH_DATABASE_URL is required")
	}
	store, e := core.Open(ctx, dsn)
	if e != nil {
		return e
	}
	defer store.Close()
	switch mode {
	case "migrate":
		return store.Migrate(ctx)
	case "import":
		if *all {
			_, e = core.ImportAll(ctx, store, *dataset)
		} else {
			_, e = core.ImportDataset(ctx, store, *dataset)
		}
		return e
	case "worker":
		return store.Work(ctx)
	case "api":
		if os.Getenv("MESH_AUTH_MODE") != "mock" && (len(userKey) < 32 || len(serviceKey) < 32 || string(userKey) == string(serviceKey)) {
			return fmt.Errorf("distinct MESH_USER_JWT_SECRET and MESH_SERVICE_JWT_SECRET of at least 32 bytes required")
		}
		handler, e := httpapi.New(store, httpapi.Auth{Mock: os.Getenv("MESH_AUTH_MODE") == "mock", Store: store, UserKey: userKey, ServiceKey: serviceKey, Issuer: issuer}, *contracts)
		if e != nil {
			return e
		}
		handler.AllowedOrigin = os.Getenv("MESH_CORS_ORIGIN")
		server := &http.Server{Addr: *addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 300 * time.Second, IdleTimeout: 60 * time.Second}
		go func() {
			<-ctx.Done()
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdown)
		}()
		slog.Info("api started", "address", *addr)
		if e = server.ListenAndServe(); e == http.ErrServerClosed {
			return nil
		}
		return e
	default:
		return fmt.Errorf("unknown mode %q", mode)
	}
}
func split(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, ",")
}


