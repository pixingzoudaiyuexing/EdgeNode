//go:build script

package js

// ContextPool 保存同一个 Isolate 下可复用的 Context。
type ContextPool struct {
	contexts chan *Context
	maxSize  int
}

func newContextPool(maxSize int) *ContextPool {
	return &ContextPool{contexts: make(chan *Context, maxSize), maxSize: maxSize}
}

func (p *ContextPool) MaxSize() int { return p.maxSize }

func (p *ContextPool) Size() int { return len(p.contexts) }

func (p *ContextPool) Get() *Context {
	select {
	case ctx := <-p.contexts:
		return ctx
	default:
		return nil
	}
}

func (p *ContextPool) Put(ctx *Context) {
	if ctx == nil {
		return
	}
	ctx.Done()
	select {
	case p.contexts <- ctx:
	default:
		ctx.Close()
	}
}

func (p *ContextPool) IsUsing() bool { return len(p.contexts) != p.maxSize }

func (p *ContextPool) Close() {
	for {
		select {
		case ctx := <-p.contexts:
			ctx.Close()
		default:
			return
		}
	}
}
