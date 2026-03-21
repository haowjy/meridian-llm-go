package llmprovider

import "context"

type Provider interface {
	GenerateResponse(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error)
	StreamResponse(ctx context.Context, req *GenerateRequest) (*Stream, error)
	Name() ProviderID
	SupportsModel(model string) bool
}
