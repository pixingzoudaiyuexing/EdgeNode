//go:build script

package js

import (
	"net/http"

	"github.com/iwind/TeaGo/maps"
)

// JSNetHTTPRequestLibrary 恢复 1.3.9 Plus 的 HTTP Request JavaScript Bridge。
type JSNetHTTPRequestLibrary struct{ JSBaseLibrary }

func init() { SharedLibraryManager.Register(&JSNetHTTPRequestLibrary{}) }

func (l *JSNetHTTPRequestLibrary) JSNamespace() string {
	return "gojs.net.http.JSNetHTTPRequestLibrary"
}

func (l *JSNetHTTPRequestLibrary) JSPrototype() string {
	return `gojs.net.http.Request = class {
	#goObjectId
	constructor() {}
	setGoObject(objectId) { this.#goObjectId = objectId }
	goObjectId() { return this.#goObjectId }
	get id() { return $this.Id(this.#goObjectId) }
	get requestId() { return this.id }
	get url() { return $this.URL(this.#goObjectId) }
	get server() { return $this.Server(this.#goObjectId) }
	get node() { return $this.Node(this.#goObjectId) }
	get requestURL() { return this.url }
	get path() { return $this.Path(this.#goObjectId) }
	get requestPath() { return this.path }
	get uri() { return $this.URI(this.#goObjectId) }
	get requestURI() { return this.uri }
	set uri(uri) { $this.SetURI(this.#goObjectId, uri) }
	set requestURI(uri) { this.uri = uri }
	get host() { return $this.Host(this.#goObjectId) }
	get remoteAddr() { return $this.RemoteAddr(this.#goObjectId) }
	get rawRemoteAddr() { return $this.RawRemoteAddr(this.#goObjectId) }
	get remotePort() { return $this.RemotePort(this.#goObjectId) }
	get method() { return $this.Method(this.#goObjectId) }
	get contentLength() { return $this.ContentLength(this.#goObjectId) }
	get transferEncoding() { return $this.TransferEncoding(this.#goObjectId) }
	get proto() { return $this.Proto(this.#goObjectId) }
	get protoMajor() { return $this.ProtoMajor(this.#goObjectId) }
	get protoMinor() { return $this.ProtoMinor(this.#goObjectId) }
	cookie(name) { return $this.Cookie(this.#goObjectId, name) }
	get header() {
		let header = $this.Header(this.#goObjectId)
		if (header == null) { header = {} }
		return header
	}
	setHeader(name, values) { $this.SetHeader(this.#goObjectId, name, values) }
	deleteHeader(name) { $this.DeleteHeader(this.#goObjectId, name) }
	setAttr(name, value) { $this.SetAttr(this.#goObjectId, name, value) }
	setVar(name, value) { $this.SetHeader(this.#goObjectId, name, value) }
	format(s) { return $this.Format(this.#goObjectId, s) }
	done() { $this.Done(this.#goObjectId) }
	close() { $this.Close(this.#goObjectId) }
	allow() { $this.Allow(this.#goObjectId) }
}`
}

func requestFromArgs(args *FunctionArguments) (RequestInterface, bool) {
	object, ok := args.GoObjectAt(0)
	if !ok {
		return nil, false
	}
	return object.(RequestInterface), true
}

func (l *JSNetHTTPRequestLibrary) Id(args *FunctionArguments) (string, error) {
	request, ok := requestFromArgs(args)
	if !ok { return "", errJSGoObjectNotFound }
	return request.Id(), nil
}
func (l *JSNetHTTPRequestLibrary) Server(args *FunctionArguments) (maps.Map, error) {
	request, ok := requestFromArgs(args)
	if !ok { return maps.Map{}, errJSGoObjectNotFound }
	return request.Server(), nil
}
func (l *JSNetHTTPRequestLibrary) Node(args *FunctionArguments) (maps.Map, error) {
	request, ok := requestFromArgs(args)
	if !ok { return maps.Map{}, errJSGoObjectNotFound }
	return request.Node(), nil
}
func (l *JSNetHTTPRequestLibrary) URL(args *FunctionArguments) (string, error) {
	request, ok := requestFromArgs(args)
	if !ok { return "", errJSGoObjectNotFound }
	return request.URL(), nil
}
func (l *JSNetHTTPRequestLibrary) Path(args *FunctionArguments) (string, error) {
	request, ok := requestFromArgs(args)
	if !ok { return "", errJSGoObjectNotFound }
	return request.Path(), nil
}
func (l *JSNetHTTPRequestLibrary) URI(args *FunctionArguments) (string, error) {
	request, ok := requestFromArgs(args)
	if !ok { return "", errJSGoObjectNotFound }
	return request.URI(), nil
}
func (l *JSNetHTTPRequestLibrary) SetURI(args *FunctionArguments) (any, error) {
	request, ok := requestFromArgs(args)
	if !ok { return l.JSGoObjectNotFound() }
	uri, _ := args.FormatStringAt(1)
	request.SetURI(uri)
	return l.JSSuccess()
}
func (l *JSNetHTTPRequestLibrary) Host(args *FunctionArguments) (string, error) {
	request, ok := requestFromArgs(args)
	if !ok { return "", errJSGoObjectNotFound }
	return request.Host(), nil
}
func (l *JSNetHTTPRequestLibrary) RemoteAddr(args *FunctionArguments) (string, error) {
	request, ok := requestFromArgs(args)
	if !ok { return "", errJSGoObjectNotFound }
	return request.RemoteAddr(), nil
}
func (l *JSNetHTTPRequestLibrary) RawRemoteAddr(args *FunctionArguments) (string, error) {
	request, ok := requestFromArgs(args)
	if !ok { return "", errJSGoObjectNotFound }
	return request.RawRemoteAddr(), nil
}
func (l *JSNetHTTPRequestLibrary) RemotePort(args *FunctionArguments) (int, error) {
	request, ok := requestFromArgs(args)
	if !ok { return 0, errJSGoObjectNotFound }
	return request.RemotePort(), nil
}
func (l *JSNetHTTPRequestLibrary) Method(args *FunctionArguments) (string, error) {
	request, ok := requestFromArgs(args)
	if !ok { return "", errJSGoObjectNotFound }
	return request.Method(), nil
}
func (l *JSNetHTTPRequestLibrary) ContentLength(args *FunctionArguments) (int64, error) {
	request, ok := requestFromArgs(args)
	if !ok { return 0, errJSGoObjectNotFound }
	return request.ContentLength(), nil
}
func (l *JSNetHTTPRequestLibrary) TransferEncoding(args *FunctionArguments) (string, error) {
	request, ok := requestFromArgs(args)
	if !ok { return "", errJSGoObjectNotFound }
	return request.TransferEncoding(), nil
}
func (l *JSNetHTTPRequestLibrary) Proto(args *FunctionArguments) (string, error) {
	request, ok := requestFromArgs(args)
	if !ok { return "", errJSGoObjectNotFound }
	return request.Proto(), nil
}
func (l *JSNetHTTPRequestLibrary) ProtoMajor(args *FunctionArguments) (int, error) {
	request, ok := requestFromArgs(args)
	if !ok { return 0, errJSGoObjectNotFound }
	return request.ProtoMajor(), nil
}
func (l *JSNetHTTPRequestLibrary) ProtoMinor(args *FunctionArguments) (int, error) {
	request, ok := requestFromArgs(args)
	if !ok { return 0, errJSGoObjectNotFound }
	return request.ProtoMinor(), nil
}
func (l *JSNetHTTPRequestLibrary) Cookie(args *FunctionArguments) (string, error) {
	request, ok := requestFromArgs(args)
	if !ok { return "", errJSGoObjectNotFound }
	name, _ := args.FormatStringAt(1)
	if name == "" { return "", nil }
	return request.Cookie(name), nil
}
func (l *JSNetHTTPRequestLibrary) Header(args *FunctionArguments) (http.Header, error) {
	request, ok := requestFromArgs(args)
	if !ok { return http.Header{}, errJSGoObjectNotFound }
	return request.Header(), nil
}
func (l *JSNetHTTPRequestLibrary) SetHeader(args *FunctionArguments) (any, error) {
	request, ok := requestFromArgs(args)
	if !ok { return l.JSGoObjectNotFound() }
	name, ok := args.FormatStringAt(1)
	if !ok || name == "" { return nil, errJSHeaderNameEmpty }
	values, valuesOK := args.FormatStringsAt(2)
	if !valuesOK {
		value, valueOK := args.FormatStringAt(2)
		if valueOK { values = []string{value} } else { values = []string{""} }
	}
	request.SetHeader(name, values)
	return l.JSSuccess()
}
func (l *JSNetHTTPRequestLibrary) DeleteHeader(args *FunctionArguments) (any, error) {
	request, ok := requestFromArgs(args)
	if !ok { return l.JSGoObjectNotFound() }
	name, ok := args.FormatStringAt(1)
	if ok { request.DeleteHeader(name) }
	return l.JSSuccess()
}
func (l *JSNetHTTPRequestLibrary) SetAttr(args *FunctionArguments) (any, error) {
	request, ok := requestFromArgs(args)
	if !ok { return l.JSGoObjectNotFound() }
	name, ok := args.FormatStringAt(1)
	if !ok || name == "" { return nil, errJSHeaderNameEmpty }
	value, _ := args.FormatStringAt(2)
	request.SetAttr(name, value)
	return l.JSSuccess()
}
func (l *JSNetHTTPRequestLibrary) SetVar(args *FunctionArguments) (any, error) {
	request, ok := requestFromArgs(args)
	if !ok { return l.JSGoObjectNotFound() }
	name, ok := args.FormatStringAt(1)
	if !ok || name == "" { return nil, errJSHeaderNameEmpty }
	value, _ := args.FormatStringAt(2)
	request.SetVar(name, value)
	return l.JSSuccess()
}
func (l *JSNetHTTPRequestLibrary) Format(args *FunctionArguments) (string, error) {
	request, ok := requestFromArgs(args)
	if !ok { return "", errJSGoObjectNotFound }
	value, _ := args.FormatStringAt(1)
	return request.Format(value), nil
}
func (l *JSNetHTTPRequestLibrary) Done(args *FunctionArguments) (any, error) {
	request, ok := requestFromArgs(args)
	if !ok { return l.JSGoObjectNotFound() }
	request.Done()
	return l.JSSuccess()
}
func (l *JSNetHTTPRequestLibrary) Close(args *FunctionArguments) (any, error) {
	request, ok := requestFromArgs(args)
	if !ok { return l.JSGoObjectNotFound() }
	request.Close()
	return l.JSSuccess()
}
func (l *JSNetHTTPRequestLibrary) Allow(args *FunctionArguments) (any, error) {
	request, ok := requestFromArgs(args)
	if !ok { return l.JSGoObjectNotFound() }
	request.Allow()
	return l.JSSuccess()
}
