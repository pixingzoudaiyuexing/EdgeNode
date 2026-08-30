//go:build script

package js

import (
	"sync/atomic"

	v8go "rogchap.com/v8go"
)

// Isolate 包装一个长期复用的 V8 Isolate 及其 ContextPool。
type Isolate struct {
	rawIsolate  *v8go.Isolate
	contextPool *ContextPool
	uses        uint32
	disposed    bool
}

func NewIsolateWithContexts(contexts int) (*Isolate, error) {
	raw := v8go.NewIsolate()
	if contexts <= 0 {
		contexts = 128
	}
	isolate := &Isolate{
		rawIsolate:  raw,
		contextPool: newContextPool(contexts),
	}
	if err := isolate.init(); err != nil {
		isolate.Dispose()
		return nil, err
	}
	return isolate, nil
}

func (i *Isolate) init() error {
	for n := 0; n < i.contextPool.MaxSize(); n++ {
		ctx, err := i.createContext()
		if err != nil {
			return err
		}
		i.contextPool.Put(ctx)
	}
	return nil
}

func (i *Isolate) OverUses() bool { return atomic.LoadUint32(&i.uses) > isolateMaxUses }

func (i *Isolate) GetContext() (*Context, error) {
	atomic.AddUint32(&i.uses, 1)
	if ctx := i.contextPool.Get(); ctx != nil {
		return ctx, nil
	}
	return i.createContext()
}

func (i *Isolate) PutContext(ctx *Context) {
	if ctx == nil {
		return
	}
	// 可信 1.3.9 Plus 的调用链会先 Done，再由 ContextPool.Put 再 Done 一次。
	ctx.Done()
	i.contextPool.Put(ctx)
}

func (i *Isolate) ContextPool() *ContextPool { return i.contextPool }

func (i *Isolate) RawIsolate() *v8go.Isolate { return i.rawIsolate }

func (i *Isolate) ThrowException(value *v8go.Value) *v8go.Value {
	return i.rawIsolate.ThrowException(value)
}

func (i *Isolate) IsUsing() bool { return i.contextPool.IsUsing() }

func (i *Isolate) Dispose() {
	if i == nil || i.disposed {
		return
	}
	i.disposed = true
	i.contextPool.Close()
	i.rawIsolate.Dispose()
}

func (i *Isolate) createContext() (*Context, error) {
	raw := v8go.NewContext(i.rawIsolate)
	return NewContext(raw, i)
}
