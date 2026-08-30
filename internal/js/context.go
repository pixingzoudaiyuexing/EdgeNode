//go:build script

package js

import (
	"sync"

	v8go "rogchap.com/v8go"
)

// Context 是可复用的请求级 V8 Context 包装。
// 字段顺序依据可信 1.3.9 Plus ABI 固定，后续 Bridge 代码依赖这些所有权关系。
type Context struct {
	rawContext     *v8go.Context
	isolate        *Isolate
	objectTemplate *ObjectTemplate
	goObjectID     uint32
	goObjectMap    map[uint32]any
	goObjectLocker sync.RWMutex
	serverID       int64
	doneCount      uint64
}

func NewContext(rawContext *v8go.Context, isolate *Isolate) (*Context, error) {
	ctx := &Context{
		rawContext:  rawContext,
		isolate:     isolate,
		goObjectMap: map[uint32]any{},
	}
	if err := ctx.init(); err != nil {
		return nil, err
	}
	return ctx, nil
}

func (c *Context) init() error {
	rawTemplate := v8go.NewObjectTemplate(c.isolate.rawIsolate)
	c.objectTemplate = &ObjectTemplate{ctx: c, raw: rawTemplate}
	return c.loadLibraries()
}

func (c *Context) loadLibraries() error {
	if _, err := c.rawContext.RunScript(bootstrapSource, "utils.js"); err != nil {
		return err
	}
	for _, library := range SharedLibraryManager.All() {
		if err := c.installLibrary(library); err != nil {
			return err
		}
	}
	return nil
}

func (c *Context) Isolate() *Isolate { return c.isolate }

func (c *Context) RawContext() *v8go.Context { return c.rawContext }

func (c *Context) NewObjectTemplate() *ObjectTemplate {
	return &ObjectTemplate{ctx: c, raw: v8go.NewObjectTemplate(c.isolate.rawIsolate)}
}

func (c *Context) RunScript(source string, origin string) (*v8go.Value, error) {
	return c.rawContext.RunScript(source, origin)
}

func (c *Context) AddGoObject(object any) uint32 {
	c.goObjectLocker.Lock()
	c.goObjectID++
	id := c.goObjectID
	c.goObjectMap[id] = object
	c.goObjectLocker.Unlock()
	return id
}

// AddGoRequestObject / AddGoResponseObject 在 1.3.9 Plus 中都是 AddGoObject 的薄封装；
// Request 与 Response 共享同一套 Context 级 Go Object ID 空间。
func (c *Context) AddGoRequestObject(object RequestInterface) uint32 { return c.AddGoObject(object) }
func (c *Context) AddGoResponseObject(object ResponseInterface) uint32 { return c.AddGoObject(object) }

func (c *Context) GoObject(id uint32) any {
	c.goObjectLocker.RLock()
	object := c.goObjectMap[id]
	c.goObjectLocker.RUnlock()
	return object
}

func (c *Context) GoObjectMap() map[uint32]any {
	c.goObjectLocker.RLock()
	objects := c.goObjectMap
	c.goObjectLocker.RUnlock()
	return objects
}

func (c *Context) Done() {
	c.doneCount++
	for _, library := range SharedLibraryManager.All() {
		library.JSDone(c)
	}

	c.goObjectLocker.Lock()
	c.goObjectID = 0
	c.goObjectMap = map[uint32]any{}
	c.goObjectLocker.Unlock()
}

func (c *Context) Cleanup() { c.Done() }

func (c *Context) Close() {
	c.Done()
	for _, library := range SharedLibraryManager.All() {
		library.JSDispose(c)
	}
	c.rawContext.Close()
}

func (c *Context) SetServerId(serverID int64) { c.serverID = serverID }

func (c *Context) ServerId() int64 { return c.serverID }
