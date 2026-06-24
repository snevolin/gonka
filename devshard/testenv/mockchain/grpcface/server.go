package grpcface

import (
	"context"
	"fmt"
	"net"

	"github.com/cosmos/cosmos-sdk/client/grpc/cmtservice"
	inferencetypes "github.com/productscience/inference/x/inference/types"
	"google.golang.org/grpc"

	"devshard/testenv/mockchain/store"
)

// Register mounts inference Query + cmtservice on srv.
func Register(srv *grpc.Server, st *store.Store) {
	inferencetypes.RegisterQueryServer(srv, NewInferenceServer(st))
	cmtservice.RegisterServiceServer(srv, NewCometServer(st))
}

// Serve listens on addr until ctx is cancelled, then graceful-stops.
func Serve(ctx context.Context, addr string, st *store.Store) error {
	if st == nil {
		return fmt.Errorf("mockchain grpc: nil store")
	}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("mockchain grpc listen %s: %w", addr, err)
	}
	srv := grpc.NewServer()
	Register(srv, st)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(lis)
	}()

	select {
	case <-ctx.Done():
		srv.GracefulStop()
		return ctx.Err()
	case err := <-errCh:
		if err == grpc.ErrServerStopped {
			return nil
		}
		return err
	}
}

// NewInProcessServer starts a gRPC server on bufconn-sized listener for tests.
// Caller must invoke cleanup on test end.
func NewInProcessServer(st *store.Store) (*grpc.Server, net.Listener, error) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	srv := grpc.NewServer()
	Register(srv, st)
	go func() { _ = srv.Serve(lis) }()
	return srv, lis, nil
}
