package dagpher

import "context"

type Endpoint func(ctx context.Context, req any) (any, error)

type Middleware func(next Endpoint) Endpoint

func Chain(middlewares ...Middleware) Middleware {
	return func(next Endpoint) Endpoint {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}
