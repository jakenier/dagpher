package dagpher

type option struct {
	mws []Middleware // middlewares to apply to all nodes in the group
}

type Option func(*option)

func defaultOption() *option {
	return &option{}
}

func getOption(opts ...Option) *option {
	opt := defaultOption()
	for _, o := range opts {
		o(opt)
	}
	return opt
}

func (o *option) mergeMws(global []Middleware) []Middleware {
	if len(global) == 0 {
		return o.mws
	}
	return append(global, o.mws...)
}

func WithMiddlewares(mws ...Middleware) Option {
	return func(o *option) {
		if len(mws) == 0 {
			return
		}
		o.mws = mws
	}
}
