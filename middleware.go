package llmprovider

import "context"

type StreamFunc func(ctx context.Context, req *GenerateRequest) (*Stream, error)

type GenerateFunc func(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error)

type ProviderCallInfo struct {
	Provider ProviderID
}

type ProviderMiddleware interface {
	WrapStream(info ProviderCallInfo, next StreamFunc) StreamFunc
	WrapGenerate(info ProviderCallInfo, next GenerateFunc) GenerateFunc
}

type WrappedProvider interface {
	Provider
	Unwrap() Provider
}

func WrapProvider(provider Provider, middleware ...ProviderMiddleware) Provider {
	if len(middleware) == 0 {
		return provider
	}

	info := ProviderCallInfo{
		Provider: provider.Name(),
	}

	streamFn := StreamFunc(provider.StreamResponse)
	generateFn := GenerateFunc(provider.GenerateResponse)

	for i := len(middleware) - 1; i >= 0; i-- {
		streamFn = middleware[i].WrapStream(info, streamFn)
		generateFn = middleware[i].WrapGenerate(info, generateFn)
	}

	return &wrappedProvider{
		base:       provider,
		streamFn:   streamFn,
		generateFn: generateFn,
	}
}

type wrappedProvider struct {
	base       Provider
	streamFn   StreamFunc
	generateFn GenerateFunc
}

func (w *wrappedProvider) GenerateResponse(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	return w.generateFn(ctx, req)
}

func (w *wrappedProvider) StreamResponse(ctx context.Context, req *GenerateRequest) (*Stream, error) {
	return w.streamFn(ctx, req)
}

func (w *wrappedProvider) Name() ProviderID {
	return w.base.Name()
}

func (w *wrappedProvider) SupportsModel(model string) bool {
	return w.base.SupportsModel(model)
}

func (w *wrappedProvider) Unwrap() Provider {
	return w.base
}
