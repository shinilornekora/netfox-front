// Package rpc — gRPC-сервер identity для gateway.
package rpc

import (
	"context"

	identityv1 "plenka/contracts/gen/identity/v1"
	"plenka/identity/internal/usecase"
)

type Server struct {
	identityv1.UnimplementedSessionsServer
	UC *usecase.Service
}

func (s *Server) Resolve(ctx context.Context, req *identityv1.ResolveRequest) (*identityv1.ResolveResponse, error) {
	a, sess, ok, err := s.UC.Resolve(ctx, req.GetToken())
	if err != nil {
		return nil, err
	}
	if !ok {
		return &identityv1.ResolveResponse{Valid: false}, nil
	}
	return &identityv1.ResolveResponse{
		Valid: true, UserId: a.ID.String(), Role: string(a.Role),
		BirthYear: int32(a.BirthYear()), ExpiresAtUnix: sess.IdleExpires.Unix(),
	}, nil
}
