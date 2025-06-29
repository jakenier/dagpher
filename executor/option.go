package executor

type option struct {
	maxGoNum int // maximum number of goroutines to run concurrently
}

type Option func(*option)

func defaultOption() *option {
	return &option{
		maxGoNum: 100, // default to 1 goroutine
	}
}

func getOption(opts ...Option) *option {
	opt := defaultOption()
	for _, o := range opts {
		o(opt)
	}
	return opt
}

func WithMaxGoNum(maxGoNum int) Option {
	return func(o *option) {
		if maxGoNum <= 0 {
			panic("maxGoNum must be greater than 0")
		}
		o.maxGoNum = maxGoNum
	}
}
