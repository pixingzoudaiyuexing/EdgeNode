//go:build script

package js

import "net/http"

// JSNetHTTPResponseLibrary 恢复 1.3.9 Plus 的 HTTP Response JavaScript Bridge。
type JSNetHTTPResponseLibrary struct{ JSBaseLibrary }

func init() { SharedLibraryManager.Register(&JSNetHTTPResponseLibrary{}) }

func (l *JSNetHTTPResponseLibrary) JSNamespace() string {
	return "gojs.net.http.JSNetHTTPResponseLibrary"
}

func (l *JSNetHTTPResponseLibrary) JSPrototype() string {
	return `gojs.net.http.Response = class {
	#goObjectId
	constructor() {}
	setGoObject(objectId) { this.#goObjectId = objectId }
	goObjectId() { return this.#goObjectId }
	setHeader(name, values) { $this.SetHeader(this.#goObjectId, name, values) }
	deleteHeader(name) { $this.DeleteHeader(this.#goObjectId, name) }
	get header() {
		let header = $this.Header(this.#goObjectId)
		if (header == null) { header = {} }
		return header
	}
	send(status, body) {
		if (body != null && typeof(body) == "object" && (body instanceof gojs.net.http.client.Response)) {
			$this.SendResp(this.#goObjectId, status, body.goObjectId)
			return
		}
		$this.Send(this.#goObjectId, status, body)
	}
	sendFile(status, path) { $this.SendFile(this.#goObjectId, status, path) }
	redirect(status, url) { $this.Redirect(this.#goObjectId, status, url) }
}`
}

func responseFromArgs(args *FunctionArguments) (ResponseInterface, bool) {
	object, ok := args.GoObjectAt(0)
	if !ok { return nil, false }
	return object.(ResponseInterface), true
}

func (l *JSNetHTTPResponseLibrary) Header(args *FunctionArguments) (http.Header, error) {
	response, ok := responseFromArgs(args)
	if !ok { return http.Header{}, errJSGoObjectNotFound }
	return response.Header(), nil
}
func (l *JSNetHTTPResponseLibrary) SetHeader(args *FunctionArguments) (any, error) {
	response, ok := responseFromArgs(args)
	if !ok { return l.JSGoObjectNotFound() }
	name, ok := args.FormatStringAt(1)
	if !ok || name == "" { return nil, errJSHeaderNameEmpty }
	values, valuesOK := args.FormatStringsAt(2)
	if !valuesOK {
		value, valueOK := args.FormatStringAt(2)
		if valueOK { values = []string{value} } else { values = []string{""} }
	}
	response.SetHeader(name, values)
	return l.JSSuccess()
}
func (l *JSNetHTTPResponseLibrary) DeleteHeader(args *FunctionArguments) (any, error) {
	response, ok := responseFromArgs(args)
	if !ok { return l.JSGoObjectNotFound() }
	name, ok := args.FormatStringAt(1)
	if ok { response.DeleteHeader(name) }
	return l.JSSuccess()
}
func (l *JSNetHTTPResponseLibrary) Send(args *FunctionArguments) (any, error) {
	response, ok := responseFromArgs(args)
	if !ok { return l.JSGoObjectNotFound() }
	status, ok := args.IntAt(1)
	if !ok { status = http.StatusOK }
	body, _ := args.FormatStringAt(2)
	response.Send(status, body)
	return l.JSSuccess()
}
func (l *JSNetHTTPResponseLibrary) SendResp(args *FunctionArguments) (int64, error) {
	response, ok := responseFromArgs(args)
	if !ok { return 0, errJSGoObjectNotFound }
	status, ok := args.IntAt(1)
	if !ok { status = http.StatusOK }
	object, ok := args.GoObjectAt(2)
	if !ok { return 0, errJSGoObjectNotFound }
	clientResponse := object.(*JSNetHTTPClientLibraryResponse)
	clientResponse.resp.StatusCode = status
	return response.SendResp(clientResponse.resp)
}
func (l *JSNetHTTPResponseLibrary) SendFile(args *FunctionArguments) (int64, error) {
	response, ok := responseFromArgs(args)
	if !ok { return 0, errJSGoObjectNotFound }
	status, ok := args.IntAt(1)
	if !ok { status = http.StatusOK }
	path, _ := args.FormatStringAt(2)
	return response.SendFile(status, path)
}
func (l *JSNetHTTPResponseLibrary) Redirect(args *FunctionArguments) (any, error) {
	response, ok := responseFromArgs(args)
	if !ok { return l.JSGoObjectNotFound() }
	status, ok := args.IntAt(1)
	if !ok { status = http.StatusFound }
	url, ok := args.StringAt(2)
	if !ok || url == "" { return nil, errJSRedirectURLRequired }
	response.Redirect(status, url)
	return l.JSSuccess()
}
