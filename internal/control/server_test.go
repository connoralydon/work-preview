package control_test

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	previewv1 "boringbison.xyz/work-preview/api/v1"
	"boringbison.xyz/work-preview/internal/control"
	"boringbison.xyz/work-preview/internal/preview"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

type rpcStore struct{ previews map[string]preview.Preview }

func (s *rpcStore) Create(_ context.Context, p preview.Preview) error {
	s.previews[p.ID] = p
	return nil
}
func (s *rpcStore) Active(context.Context) ([]preview.Preview, error) {
	var result []preview.Preview
	for _, p := range s.previews {
		if p.Status == preview.StatusActive {
			result = append(result, p)
		}
	}
	return result, nil
}
func (s *rpcStore) GetActive(_ context.Context, id string) (preview.Preview, error) {
	p, ok := s.previews[id]
	if !ok || p.Status != preview.StatusActive {
		return preview.Preview{}, preview.ErrNotFound
	}
	return p, nil
}
func (s *rpcStore) Touch(context.Context, string, time.Time, time.Time) error { return nil }
func (s *rpcStore) SetStatus(_ context.Context, id, status string, _ time.Time) error {
	p := s.previews[id]
	p.Status = status
	s.previews[id] = p
	return nil
}

type rpcReloader struct{ calls int }

func (r *rpcReloader) Reload(context.Context) error { r.calls++; return nil }

func TestGRPCCreateListAndDelete(t *testing.T) {
	root := t.TempDir()
	store := &rpcStore{previews: map[string]preview.Preview{}}
	reloader := &rpcReloader{}
	manager := &preview.Manager{
		Store:    store,
		Files:    preview.CaddyWriter{SnippetDir: filepath.Join(root, "caddy"), LogDir: filepath.Join(root, "logs"), Domain: "p.boringbison.xyz", Certificate: "/cert", CertificateKey: "/key"},
		Reloader: reloader, TTL: time.Hour,
	}
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	previewv1.RegisterPreviewServiceServer(server, &control.Server{Manager: manager, Domain: "p.boringbison.xyz"})
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := previewv1.NewPreviewServiceClient(conn)
	created, err := client.CreatePreview(context.Background(), &previewv1.CreatePreviewRequest{Prefix: "grpc", Port: 4321})
	if err != nil {
		t.Fatal(err)
	}
	if created.Url != "https://grpc.p.boringbison.xyz" || created.ExpiresAt.AsTime().Sub(created.CreatedAt.AsTime()) != time.Hour {
		t.Fatalf("unexpected preview: %+v", created)
	}
	listed, err := client.ListPreviews(context.Background(), &emptypb.Empty{})
	if err != nil || len(listed.Previews) != 1 {
		t.Fatalf("list: %+v, %v", listed, err)
	}
	if _, err := client.DeletePreview(context.Background(), &previewv1.DeletePreviewRequest{Id: created.Id}); err != nil {
		t.Fatal(err)
	}
	listed, err = client.ListPreviews(context.Background(), &emptypb.Empty{})
	if err != nil || len(listed.Previews) != 0 {
		t.Fatalf("list after delete: %+v, %v", listed, err)
	}
	if reloader.calls != 2 {
		t.Fatalf("reloads=%d, want create and delete", reloader.calls)
	}
}
