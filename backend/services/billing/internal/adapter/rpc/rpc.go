// Package rpc — gRPC Access.Check для playback.
package rpc

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"plenka/billing/internal/usecase"
	billingv1 "plenka/contracts/gen/billing/v1"
)

type Server struct {
	billingv1.UnimplementedAccessServer
	Account *usecase.Account
}

func (s *Server) Check(ctx context.Context, req *billingv1.CheckRequest) (*billingv1.CheckResponse, error) {
	user, err1 := uuid.Parse(req.GetUserId())
	title, err2 := uuid.Parse(req.GetTitleId())
	if err1 != nil || err2 != nil {
		return nil, status.Error(codes.InvalidArgument, "user_id and title_id must be UUIDs")
	}
	d, _, _, _, _, err := s.Account.Check(ctx, user, title)
	if err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	resp := &billingv1.CheckResponse{Allowed: d.Allowed}
	switch d.Via {
	case "subscription":
		resp.Grant = billingv1.Grant_GRANT_SUBSCRIPTION
	case "purchase":
		resp.Grant = billingv1.Grant_GRANT_PURCHASE
	}
	switch d.Denial {
	case "not_entitled":
		resp.Denial = billingv1.Denial_DENIAL_NOT_ENTITLED
	case "not_available":
		resp.Denial = billingv1.Denial_DENIAL_NOT_AVAILABLE
	}
	if !d.ValidUntil.IsZero() {
		resp.ValidUntilUnix = d.ValidUntil.Unix()
	}
	return resp, nil
}
