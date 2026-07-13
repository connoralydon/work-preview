package control

import (
	"context"
	"errors"
	"fmt"

	previewv1 "github.com/connoralydon/work-preview/api/v1"
	"github.com/connoralydon/work-preview/internal/preview"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
	previewv1.UnimplementedPreviewServiceServer
	Manager *preview.Manager
	Domain  string
}

func (s *Server) CreatePreview(ctx context.Context, req *previewv1.CreatePreviewRequest) (*previewv1.Preview, error) {
	p, err := s.Manager.Create(ctx, req.GetPrefix(), req.GetPort(), preview.Source{
		Repository: req.GetRepository(), Branch: req.GetBranch(), Commit: req.GetCommit(),
	})
	if err != nil {
		return nil, rpcError(err)
	}
	return s.proto(p), nil
}

func (s *Server) DeletePreview(ctx context.Context, req *previewv1.DeletePreviewRequest) (*emptypb.Empty, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if err := s.Manager.Delete(ctx, req.GetId()); err != nil {
		return nil, rpcError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) ListPreviews(ctx context.Context, _ *emptypb.Empty) (*previewv1.ListPreviewsResponse, error) {
	previews, err := s.Manager.Active(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	response := &previewv1.ListPreviewsResponse{Previews: make([]*previewv1.Preview, 0, len(previews))}
	for _, p := range previews {
		response.Previews = append(response.Previews, s.proto(p))
	}
	return response, nil
}

func (s *Server) proto(p preview.Preview) *previewv1.Preview {
	return &previewv1.Preview{
		Id: p.ID, Prefix: p.Prefix, Port: uint32(p.Port),
		Url:          fmt.Sprintf("https://%s.%s", p.Prefix, s.Domain),
		CreatedAt:    timestamppb.New(p.CreatedAt),
		LastAccessAt: timestamppb.New(p.LastAccessAt),
		ExpiresAt:    timestamppb.New(p.ExpiresAt),
		Repository:   p.Repository,
		Branch:       p.Branch,
		Commit:       p.Commit,
	}
}

func rpcError(err error) error {
	switch {
	case errors.Is(err, preview.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, preview.ErrPrefixConflict):
		return status.Error(codes.AlreadyExists, err.Error())
	default:
		return status.Error(codes.InvalidArgument, err.Error())
	}
}
