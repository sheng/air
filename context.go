package air

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

// Context represents the context of the current HTTP request. It holds request and
// response objects, path, path parameters, data and registered handler.
type Context struct {
	goContext context.Context

	Request      *Request
	Response     *Response
	PristinePath string
	ParamNames   []string
	Params       map[string]string
	Handler      HandlerFunc
	StatusCode   int
	Data         JSONMap
	Air          *Air
}

// newContext returns a new instance of `Context`.
func newContext(a *Air) *Context {
	return &Context{
		goContext:  context.Background(),
		Params:     make(map[string]string),
		Handler:    NotFoundHandler,
		StatusCode: http.StatusOK,
		Data:       make(JSONMap),
		Air:        a,
	}
}

// Deadline returns the time when work done on behalf of this context
// should be canceled. Deadline returns ok==false when no deadline is
// set. Successive calls to Deadline return the same results.
func (c *Context) Deadline() (deadline time.Time, ok bool) {
	return c.goContext.Deadline()
}

// Done returns a channel that's closed when work done on behalf of this
// context should be canceled. Done may return nil if this context can
// never be canceled. Successive calls to Done return the same value.
func (c *Context) Done() <-chan struct{} {
	return c.goContext.Done()
}

// Err returns a non-nil error value after Done is closed. Err returns
// Canceled if the context was canceled or DeadlineExceeded if the
// context's deadline passed. No other values for Err are defined.
// After Done is closed, successive calls to Err return the same value.
func (c *Context) Err() error {
	return c.goContext.Err()
}

// Value returns the value associated with this context for key, or nil
// if no value is associated with key. Successive calls to Value with
// the same key returns the same result.
func (c *Context) Value(key interface{}) interface{} {
	return c.goContext.Value(key)
}

// SetValue sets request-scoped value into the context.
func (c *Context) SetValue(key interface{}, val interface{}) {
	c.goContext = context.WithValue(c.goContext, key, val)
}

// QueryParam returns the query param for the provided name. It is an alias
// for `URI#QueryParam()`.
func (c *Context) QueryParam(name string) string {
	return c.Request.URI.QueryParam(name)
}

// QueryParams returns the query parameters as map. It is an alias for
// `URI#QueryParams()`.
func (c *Context) QueryParams() map[string][]string {
	return c.Request.URI.QueryParams()
}

// FormValue returns the form field value for the provided name. It is an
// alias for `Request#FormValue()`.
func (c *Context) FormValue(name string) string {
	return c.Request.FormValue(name)
}

// FormParams returns the form parameters as map.
// It is an alias for `Request#FormParams()`.
func (c *Context) FormParams() map[string][]string {
	return c.Request.FormParams()
}

// FormFile returns the multipart form file for the provided name. It is an
// alias for `Request#FormFile()`.
func (c *Context) FormFile(name string) (*multipart.FileHeader, error) {
	return c.Request.FormFile(name)
}

// MultipartForm returns the multipart form.
// It is an alias for `Request#MultipartForm()`.
func (c *Context) MultipartForm() (*multipart.Form, error) {
	return c.Request.MultipartForm()
}

// Cookie returns the named cookie provided in the request.
// It is an alias for `Request#Cookie()`.
func (c *Context) Cookie(name string) (*Cookie, error) {
	return c.Request.Cookie(name)
}

// SetCookie adds a "Set-Cookie" header in HTTP response.
// It is an alias for `Response#SetCookie()`.
func (c *Context) SetCookie(cookie Cookie) {
	c.Response.SetCookie(cookie)
}

// Cookies returns the HTTP cookies sent with the request. It is an alias
// for `Request#Cookies()`.
func (c *Context) Cookies() []Cookie {
	return c.Request.Cookies()
}

// Bind binds the request body into provided type i. The default binder doe
// it based on "Content-Type" header.
func (c *Context) Bind(i interface{}) error {
	return c.Air.binder.bind(i, c)
}

// Render renders a template with `Context#Data` and `Context#Data["template"]`
// and sends a "text/html" response with `Context#StatusCode`.
func (c *Context) Render() error {
	t, ok := c.Data["template"]
	if !ok || reflect.ValueOf(t).Kind() != reflect.String {
		return errors.New("c.Data[\"template\"] not setted")
	}
	buf := &bytes.Buffer{}
	if err := c.Air.renderer.render(buf, t.(string), c); err != nil {
		return err
	}
	c.Response.Header.Set(HeaderContentType, MIMETextHTML)
	c.Response.WriteHeader(c.StatusCode)
	_, err := c.Response.Write(buf.Bytes())
	return err
}

// HTML sends an HTTP response with `Context#StatusCode` and `Context#Data["html"]`.
func (c *Context) HTML() error {
	h, ok := c.Data["html"]
	if !ok || reflect.ValueOf(h).Kind() != reflect.String {
		return errors.New("c.Data[\"html\"] not setted")
	}
	c.Response.Header.Set(HeaderContentType, MIMETextHTML)
	c.Response.WriteHeader(c.StatusCode)
	_, err := c.Response.Write([]byte(h.(string)))
	return err
}

// String sends a string response with `Context#StatusCode` and `Context#Data["string"]`.
func (c *Context) String() error {
	s, ok := c.Data["string"]
	if !ok || reflect.ValueOf(s).Kind() != reflect.String {
		return errors.New("c.Data[\"string\"] not setted")
	}
	c.Response.Header.Set(HeaderContentType, MIMETextPlain)
	c.Response.WriteHeader(c.StatusCode)
	_, err := c.Response.Write([]byte(s.(string)))
	return err
}

// JSON sends a JSON response with `Context#StatusCode` and `Context#Data["json"]`.
func (c *Context) JSON() error {
	j, ok := c.Data["json"]
	if !ok {
		return errors.New("c.Data[\"json\"] not setted")
	}
	b, err := json.Marshal(j)
	if c.Air.Config.DebugMode {
		b, err = json.MarshalIndent(j, "", "\t")
	}
	if err != nil {
		return err
	}
	return c.JSONBlob(b)
}

// JSONBlob sends a JSON blob response with `Context#StatusCode`.
func (c *Context) JSONBlob(b []byte) error {
	return c.Blob(MIMEApplicationJSON, b)
}

// JSONP sends a JSONP response with `Context#StatusCode` and `Context#Data["jsonp"]`.
// It uses `Context#Data["callback"]` to construct the JSONP payload.
func (c *Context) JSONP() error {
	j, jok := c.Data["jsonp"]
	if !jok {
		return errors.New("c.Data[\"jsonp\"] not setted")
	}
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return c.JSONPBlob(b)
}

// JSONPBlob sends a JSONP blob response with `Context#StatusCode`. It uses
// `Context#Data["callback"]` to construct the JSONP payload.
func (c *Context) JSONPBlob(b []byte) error {
	cb, cbok := c.Data["callback"]
	if !cbok || reflect.ValueOf(cb).Kind() != reflect.String {
		return errors.New("c.Data[\"callback\"] not setted")
	}
	c.Response.Header.Set(HeaderContentType, MIMEApplicationJavaScript)
	c.Response.WriteHeader(c.StatusCode)
	if _, err := c.Response.Write([]byte(cb.(string) + "(")); err != nil {
		return err
	}
	if _, err := c.Response.Write(b); err != nil {
		return err
	}
	_, err := c.Response.Write([]byte(");"))
	return err
}

// XML sends an XML response with `Context#StatusCode` and `Context#Data["xml"]`.
func (c *Context) XML() error {
	x, ok := c.Data["xml"]
	if !ok {
		return errors.New("c.Data[\"xml\"] not setted")
	}
	b, err := xml.Marshal(x)
	if c.Air.Config.DebugMode {
		b, err = xml.MarshalIndent(x, "", "\t")
	}
	if err != nil {
		return err
	}
	return c.XMLBlob(b)
}

// XMLBlob sends a XML blob response with `Context#StatusCode`.
func (c *Context) XMLBlob(b []byte) error {
	if _, err := c.Response.Write([]byte(xml.Header)); err != nil {
		return err
	}
	return c.Blob(MIMEApplicationXML, b)
}

// Blob sends a blob response with `Context#StatusCode` and contentType.
func (c *Context) Blob(contentType string, b []byte) error {
	c.Response.Header.Set(HeaderContentType, contentType)
	c.Response.WriteHeader(c.StatusCode)
	_, err := c.Response.Write(b)
	return err
}

// Stream sends a streaming response with `Context#StatusCode` and contentType.
func (c *Context) Stream(contentType string, r io.Reader) error {
	c.Response.Header.Set(HeaderContentType, contentType)
	c.Response.WriteHeader(c.StatusCode)
	_, err := io.Copy(c.Response, r)
	return err
}

// File sends a response with the content of the file.
func (c *Context) File(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return ErrNotFound
	}
	defer f.Close()

	fi, _ := f.Stat()
	if fi.IsDir() {
		file = filepath.Join(file, "index.html")
		f, err = os.Open(file)
		if err != nil {
			return ErrNotFound
		}
		if fi, err = f.Stat(); err != nil {
			return err
		}
	}
	return c.ServeContent(f, fi.Name(), fi.ModTime())
}

// Attachment sends a response from `io.ReaderSeeker` as attachment, prompting
// client to save the file.
func (c *Context) Attachment(r io.ReadSeeker, name string) error {
	return c.contentDisposition(r, name, "attachment")
}

// Inline sends a response from `io.ReaderSeeker` as inline, opening the
// file in the browser.
func (c *Context) Inline(r io.ReadSeeker, name string) error {
	return c.contentDisposition(r, name, "inline")
}

// contentDisposition sends a response from `io.ReaderSeeker` as dispositionType.
func (c *Context) contentDisposition(r io.ReadSeeker, name, dispositionType string) error {
	c.Response.Header.Set(HeaderContentType, contentTypeByExtension(name))
	c.Response.Header.Set(HeaderContentDisposition, dispositionType+"; filename="+name)
	c.Response.WriteHeader(http.StatusOK)
	_, err := io.Copy(c.Response, r)
	return err
}

// NoContent sends a response with no body and a `Context#StatusCode`.
func (c *Context) NoContent() error {
	c.Response.WriteHeader(c.StatusCode)
	return nil
}

// Redirect redirects the request with `Context#StatusCode`.
func (c *Context) Redirect(uri string) error {
	if c.StatusCode < http.StatusMultipleChoices || c.StatusCode > http.StatusTemporaryRedirect {
		return ErrInvalidRedirectCode
	}
	c.Response.Header.Set(HeaderLocation, uri)
	c.Response.WriteHeader(c.StatusCode)
	return nil
}

// ServeContent sends static content from `io.Reader` and handles caching
// via "If-Modified-Since" request header. It automatically sets "Content-Type"
// and "Last-Modified" response headers.
func (c *Context) ServeContent(content io.ReadSeeker, name string, modtime time.Time) error {
	req := c.Request
	res := c.Response

	if t, err := time.Parse(http.TimeFormat, req.Header.Get(HeaderIfModifiedSince)); err == nil && modtime.Before(t.Add(1*time.Second)) {
		res.Header.Del(HeaderContentType)
		res.Header.Del(HeaderContentLength)
		c.StatusCode = http.StatusNotModified
		return c.NoContent()
	}

	res.Header.Set(HeaderContentType, contentTypeByExtension(name))
	res.Header.Set(HeaderLastModified, modtime.UTC().Format(http.TimeFormat))
	res.WriteHeader(http.StatusOK)
	_, err := io.Copy(res, content)
	return err
}

// reset resets the instance of `Context`.
func (c *Context) reset() {
	c.goContext = context.Background()
	c.Request.reset()
	c.Response.reset()
	c.PristinePath = ""
	c.ParamNames = c.ParamNames[:0]
	for k := range c.Params {
		delete(c.Params, k)
	}
	c.Handler = NotFoundHandler
	c.StatusCode = http.StatusOK
	for k := range c.Data {
		delete(c.Data, k)
	}
}

// contentTypeByExtension returns the MIME type associated with the file based
// on its extension. It returns "application/octet-stream" incase MIME type is
// not found.
func contentTypeByExtension(name string) string {
	t := mime.TypeByExtension(filepath.Ext(name))
	if t == "" {
		t = MIMEOctetStream
	}
	return t
}
