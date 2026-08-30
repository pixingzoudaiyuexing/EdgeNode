//go:build script

package js

import (
	"time"

	"github.com/TeaOSLab/EdgeNode/internal/remotelogs"
	"github.com/TeaOSLab/EdgeNode/internal/utils/goman"
	memutils "github.com/TeaOSLab/EdgeNode/internal/utils/mem"
)

const (
	defaultContextsPerIsolate = 8
	isolateMaxUses            = 4096
	isolatePoolTickInterval   = 5 * time.Second
)

// IsolatePool 使用固定活跃池和 dirty 队列渐进替换过度使用的 Isolate。
type IsolatePool struct {
	isolates  []*Isolate
	dirty     []*Isolate
	ticker    *time.Ticker
	tickIndex int
	size      int
	current   int
}

func NewIsolatePool(count int) (*IsolatePool, error) {
	if count <= 0 {
		count = 1
	} else if count > 512 {
		count = 512
	}
	pool := &IsolatePool{isolates: make([]*Isolate, count), size: count}
	if err := pool.init(); err != nil {
		for _, isolate := range pool.isolates {
			if isolate != nil {
				isolate.Dispose()
			}
		}
		return nil, err
	}
	return pool, nil
}

func NewAutoIsolatePool() (*IsolatePool, error) {
	count := memutils.SystemMemoryGB() * 2
	if count < 2 {
		count = 2
	} else if count > 256 {
		count = 256
	}
	return NewIsolatePool(count)
}

func (p *IsolatePool) init() error {
	for index := range p.isolates {
		isolate, err := NewIsolateWithContexts(defaultContextsPerIsolate)
		if err != nil {
			return err
		}
		p.isolates[index] = isolate
	}

	// 原版会借一个 Context 执行 gojs.runOnce()，失败只记录日志，不阻止池启动。
	ctx, err := p.GetContext()
	if err != nil {
		remotelogs.Error("JS", "获取 JS 初始化 Context 失败: "+err.Error())
	} else if ctx != nil {
		if _, runErr := ctx.RunScript("gojs.runOnce()", "utils.js"); runErr != nil {
			remotelogs.Error("JS", "执行 JS 初始化自检失败: "+runErr.Error())
		}
		p.PutContext(ctx)
	}

	p.ticker = time.NewTicker(isolatePoolTickInterval)
	goman.New(func() {
		for range p.ticker.C {
			p.tick()
		}
	})
	return nil
}

func (p *IsolatePool) tick() {
	removed := 0
	for removed < len(p.dirty) {
		isolate := p.dirty[removed]
		if isolate.IsUsing() {
			break
		}
		isolate.Dispose()
		removed++
	}
	if removed > 0 {
		p.dirty = p.dirty[removed:]
	}

	if len(p.isolates) == 0 {
		return
	}
	if p.tickIndex >= len(p.isolates) {
		p.tickIndex = 0
	}
	index := p.tickIndex
	isolate := p.isolates[index]
	p.tickIndex++
	if !isolate.OverUses() {
		return
	}

	replacement, err := NewIsolateWithContexts(defaultContextsPerIsolate)
	if err != nil {
		remotelogs.Error("JS", "替换过度使用的 V8 Isolate 失败: "+err.Error())
		return
	}
	p.isolates[index] = replacement
	p.dirty = append(p.dirty, isolate)
}

func (p *IsolatePool) GetContext() (*Context, error) {
	if p == nil || p.size == 0 {
		return nil, nil
	}
	p.current++
	if p.current >= p.size {
		p.current = 0
	}
	return p.isolates[p.current].GetContext()
}

func (p *IsolatePool) PutContext(ctx *Context) {
	if ctx == nil || ctx.Isolate() == nil {
		return
	}
	ctx.Isolate().PutContext(ctx)
}
