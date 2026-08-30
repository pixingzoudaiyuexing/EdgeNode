//go:build script

package js

import (
	"sync/atomic"
	"testing"
)

func TestBootstrapAndContextReuse(t *testing.T) {
	isolate, err := NewIsolateWithContexts(1)
	if err != nil {
		t.Fatal(err)
	}
	defer isolate.Dispose()

	ctx, err := isolate.GetContext()
	if err != nil {
		t.Fatal(err)
	}
	if ctx == nil {
		t.Fatal("expected context")
	}

	value, err := ctx.RunScript(`
		gojs.prepareNamespace("gojs.net.http");
		let inherited = {skip: 1};
		let source = Object.create(inherited);
		source.keep = 2;
		let dest = {};
		gojs.copyAttrs(dest, source);
		let count = 0;
		gojs.once(function () { count++; });
		gojs.runOnce();
		gojs.runOnce();
		JSON.stringify({namespace: typeof gojs.net.http, copied: dest.keep, skipped: dest.skip, count: count});
	`, "bootstrap-test.js")
	if err != nil {
		t.Fatal(err)
	}
	if got := value.String(); got != `{"namespace":"object","copied":2,"count":2}` {
		t.Fatalf("unexpected bootstrap result: %s", got)
	}

	if id := ctx.AddGoObject("first"); id != 1 {
		t.Fatalf("first object id = %d, want 1", id)
	}
	if id := ctx.AddGoObject("second"); id != 2 {
		t.Fatalf("second object id = %d, want 2", id)
	}
	if got := ctx.GoObject(2); got != "second" {
		t.Fatalf("unexpected object: %#v", got)
	}

	isolate.PutContext(ctx)
	reused, err := isolate.GetContext()
	if err != nil {
		t.Fatal(err)
	}
	if reused != ctx {
		t.Fatal("expected pooled context to be reused")
	}
	if got := reused.GoObject(1); got != nil {
		t.Fatalf("go object map was not reset: %#v", got)
	}
	if id := reused.AddGoObject("again"); id != 1 {
		t.Fatalf("object id after Done = %d, want 1", id)
	}
	isolate.PutContext(reused)
}

func TestOverUsesBoundary(t *testing.T) {
	isolate := &Isolate{}
	atomic.StoreUint32(&isolate.uses, isolateMaxUses)
	if isolate.OverUses() {
		t.Fatal("4096 uses must not be over limit")
	}
	atomic.StoreUint32(&isolate.uses, isolateMaxUses+1)
	if !isolate.OverUses() {
		t.Fatal("4097 uses must be over limit")
	}
}

func TestTickReplacesAndDefersDirtyDispose(t *testing.T) {
	old, err := NewIsolateWithContexts(1)
	if err != nil {
		t.Fatal(err)
	}

	held, err := old.GetContext()
	if err != nil {
		old.Dispose()
		t.Fatal(err)
	}
	atomic.StoreUint32(&old.uses, isolateMaxUses+1)

	pool := &IsolatePool{isolates: []*Isolate{old}, size: 1}
	pool.tick()
	if pool.isolates[0] == old {
		old.PutContext(held)
		old.Dispose()
		t.Fatal("expected active isolate replacement")
	}
	if len(pool.dirty) != 1 || pool.dirty[0] != old {
		t.Fatal("old isolate was not moved to dirty queue")
	}
	if old.disposed {
		t.Fatal("in-use dirty isolate must not be disposed")
	}

	pool.tick()
	if old.disposed {
		t.Fatal("dirty isolate was disposed while context was still held")
	}

	old.PutContext(held)
	pool.tick()
	if !old.disposed {
		t.Fatal("idle dirty isolate should be disposed")
	}
	if len(pool.dirty) != 0 {
		t.Fatalf("dirty queue size = %d, want 0", len(pool.dirty))
	}

	pool.isolates[0].Dispose()
}

type lifecycleLibrary struct {
	JSBaseLibrary
	initCount    int
	doneCount    int
	disposeCount int
}

func (l *lifecycleLibrary) JSNamespace() string { return "gojs.test" }
// 这里只测试生命周期；真实 loader 会执行 Prototype，所以不要使用旧占位符 "Test"。
func (l *lifecycleLibrary) JSPrototype() string { return "" }
func (l *lifecycleLibrary) JSInit(ctx *Context)  { l.initCount++ }
func (l *lifecycleLibrary) JSDone(ctx *Context)  { l.doneCount++ }
func (l *lifecycleLibrary) JSDispose(ctx *Context) {
	l.disposeCount++
}

func TestLibraryLifecycleAndDoubleDone(t *testing.T) {
	original := SharedLibraryManager
	manager := &LibraryManager{}
	library := &lifecycleLibrary{}
	manager.Register(library)
	SharedLibraryManager = manager
	defer func() { SharedLibraryManager = original }()

	isolate, err := NewIsolateWithContexts(1)
	if err != nil {
		t.Fatal(err)
	}
	if library.initCount != 1 {
		t.Fatalf("JSInit count = %d, want 1", library.initCount)
	}
	// 初始化 Context 入池时 ContextPool.Put 会调用一次 Done。
	if library.doneCount != 1 {
		t.Fatalf("initial JSDone count = %d, want 1", library.doneCount)
	}

	ctx, err := isolate.GetContext()
	if err != nil {
		isolate.Dispose()
		t.Fatal(err)
	}
	isolate.PutContext(ctx)
	// Isolate.PutContext + ContextPool.Put 按原版连续调用两次 Done。
	if library.doneCount != 3 {
		isolate.Dispose()
		t.Fatalf("JSDone count after PutContext = %d, want 3", library.doneCount)
	}

	isolate.Dispose()
	if library.doneCount != 4 {
		t.Fatalf("JSDone count after Dispose = %d, want 4", library.doneCount)
	}
	if library.disposeCount != 1 {
		t.Fatalf("JSDispose count = %d, want 1", library.disposeCount)
	}
}
