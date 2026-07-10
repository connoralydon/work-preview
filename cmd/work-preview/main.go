package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	previewv1 "boringbison.xyz/work-preview/api/v1"
	"boringbison.xyz/work-preview/internal/control"
	"boringbison.xyz/work-preview/internal/preview"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

const defaultSocket = "/run/work-preview/control.sock"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = serve(os.Args[2:])
	case "expose":
		err = expose(os.Args[2:])
	case "delete":
		err = deletePreview(os.Args[2:])
	case "list":
		err = listPreviews(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "work-preview:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: work-preview <serve|expose|delete|list> [options]")
	os.Exit(2)
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	database := fs.String("database", "/var/lib/work-preview/work-preview.db", "SQLite database file")
	domain := fs.String("domain", "p.boringbison.xyz", "preview domain suffix")
	socket := fs.String("socket", defaultSocket, "gRPC Unix socket")
	snippetDir := fs.String("snippet-dir", "/run/work-preview/caddy", "generated Caddyfile directory")
	logDir := fs.String("log-dir", "/var/log/work-preview", "Caddy access log directory")
	certificate := fs.String("tls-cert", "", "Cloudflare origin certificate path")
	certificateKey := fs.String("tls-key", "", "Cloudflare origin certificate key path")
	caddyfile := fs.String("caddyfile", "/etc/caddy/caddy_config", "root Caddyfile importing generated snippets")
	caddyBin := fs.String("caddy-bin", "caddy", "Caddy executable")
	caddyAddress := fs.String("caddy-address", "", "Caddy admin API address")
	ttl := fs.Duration("ttl", time.Hour, "preview inactivity TTL")
	sweepInterval := fs.Duration("sweep-interval", time.Minute, "access-log and expiry scan interval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *database == "" || *certificate == "" || *certificateKey == "" {
		return errors.New("database, tls-cert, and tls-key are required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	store, err := preview.OpenSQLite(ctx, *database)
	if err != nil {
		return err
	}
	defer store.Close()
	manager := &preview.Manager{
		Store: store,
		Files: preview.CaddyWriter{
			SnippetDir: *snippetDir, LogDir: *logDir, Domain: *domain,
			Certificate: *certificate, CertificateKey: *certificateKey,
		},
		Reloader: preview.CommandReloader{Binary: *caddyBin, ConfigFile: *caddyfile, Address: *caddyAddress},
		TTL:      *ttl,
	}
	if err := manager.Reconcile(ctx); err != nil {
		return fmt.Errorf("reconcile previews: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(*socket), 0o750); err != nil {
		return err
	}
	_ = os.Remove(*socket)
	listener, err := net.Listen("unix", *socket)
	if err != nil {
		return err
	}
	defer os.Remove(*socket)
	if err := os.Chmod(*socket, 0o660); err != nil {
		listener.Close()
		return err
	}
	grpcServer := grpc.NewServer()
	previewv1.RegisterPreviewServiceServer(grpcServer, &control.Server{Manager: manager, Domain: *domain})
	go sweep(ctx, manager, *sweepInterval)
	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()
	return grpcServer.Serve(listener)
}

func sweep(ctx context.Context, manager *preview.Manager, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := manager.Sweep(ctx); err != nil {
				fmt.Fprintln(os.Stderr, "work-preview: sweep:", err)
			}
		}
	}
}

func expose(args []string) error {
	fs := flag.NewFlagSet("expose", flag.ContinueOnError)
	port := fs.Uint("port", 0, "loopback dev-server port")
	prefix := fs.String("prefix", "", "optional hostname prefix")
	socket := fs.String("socket", defaultSocket, "gRPC Unix socket")
	jsonOutput := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	conn, client, err := client(*socket)
	if err != nil {
		return err
	}
	defer conn.Close()
	p, err := client.CreatePreview(context.Background(), &previewv1.CreatePreviewRequest{Port: uint32(*port), Prefix: *prefix})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"id": p.Id, "url": p.Url})
	}
	fmt.Printf("%s\t%s\n", p.Id, p.Url)
	return nil
}

func deletePreview(args []string) error {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	socket := fs.String("socket", defaultSocket, "gRPC Unix socket")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("delete requires a preview ID")
	}
	conn, client, err := client(*socket)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = client.DeletePreview(context.Background(), &previewv1.DeletePreviewRequest{Id: fs.Arg(0)})
	return err
}

func listPreviews(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	socket := fs.String("socket", defaultSocket, "gRPC Unix socket")
	jsonOutput := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	conn, client, err := client(*socket)
	if err != nil {
		return err
	}
	defer conn.Close()
	response, err := client.ListPreviews(context.Background(), &emptypb.Empty{})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(response.Previews)
	}
	for _, p := range response.Previews {
		fmt.Printf("%s\t%s\t%d\t%s\n", p.Id, p.Url, p.Port, p.ExpiresAt.AsTime().Format(time.RFC3339))
	}
	return nil
}

func client(socket string) (*grpc.ClientConn, previewv1.PreviewServiceClient, error) {
	conn, err := grpc.NewClient("unix://"+socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	return conn, previewv1.NewPreviewServiceClient(conn), nil
}
