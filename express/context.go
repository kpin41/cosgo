package express

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Context struct {
	index     uint8
	query     url.Values
	params    map[string]string
	matchPath *MPath

	Path     string
	Engine   *Engine
	Routes   []string //当前匹配到的全部路由
	Request  *http.Request
	Response *Response
}

// context returns a Context instance.
func NewContext(e *Engine, r *http.Request, w http.ResponseWriter) *Context {
	return &Context{
		Engine:   e,
		Request:  r,
		Response: NewResponse(w, e),
	}
}

const (
	indexPage     = "index.html"
	defaultMemory = 32 << 20 // 32 MB
)

func (c *Context) writeContentType(value string) {
	header := c.Response.Header()
	if header.Get(HeaderContentType) == "" {
		header.Set(HeaderContentType, value)
	}
}

// Next should be used only inside middleware.
func (c *Context) Next() {
	index := c.index
	c.index++
	middlewareNum := uint8(len(c.Engine.middleware))
	if index < middlewareNum {
		h := c.Engine.middleware[index]
		h(c)
	} else {
		i := index - middlewareNum
		if i < uint8(len(c.Engine.router.routes)) {
			c.match(i)
		}
	}

}

func (c *Context) match(i uint8) {
	//Find Router
	route := c.Engine.router.routes[i]
	if params, ok := route.Match(c.Request.Method, c.matchPath); ok {
		c.params = params
		err := route.Handler(c)
		if err != nil {
			c.Engine.HTTPErrorHandler(c, err)
			return
		}
	}
}

func (c *Context) IsTLS() bool {
	return c.Request.TLS != nil
}

func (c *Context) IsWebSocket() bool {
	upgrade := c.Request.Header.Get(HeaderUpgrade)
	return strings.ToLower(upgrade) == "websocket"
}

//设置状态码
func (c *Context) Status(code int) *Context {
	c.Response.Status(code)
	return c
}

//协议
func (c *Context) Protocol() string {
	// Can't use `r.Request.URL.Protocol`
	// See: https://groups.google.com/forum/#!topic/golang-nuts/pMUkBlQBDF0
	if c.IsTLS() {
		return "https"
	}
	if scheme := c.Request.Header.Get(HeaderXForwardedProto); scheme != "" {
		return scheme
	}
	if scheme := c.Request.Header.Get(HeaderXForwardedProtocol); scheme != "" {
		return scheme
	}
	if ssl := c.Request.Header.Get(HeaderXForwardedSsl); ssl == "on" {
		return "https"
	}
	if scheme := c.Request.Header.Get(HeaderXUrlScheme); scheme != "" {
		return scheme
	}
	return "http"
}

func (c *Context) RemoteAddr() string {
	if c.Engine != nil && c.Engine.IPExtractor != nil {
		return c.Engine.IPExtractor(c.Request)
	}
	// Fall back to legacy behavior
	if ip := c.Request.Header.Get(HeaderXForwardedFor); ip != "" {
		return strings.Split(ip, ", ")[0]
	}
	if ip := c.Request.Header.Get(HeaderXRealIP); ip != "" {
		return ip
	}
	ra, _, _ := net.SplitHostPort(c.Request.RemoteAddr)
	return ra
}

func (c *Context) Param(name string) string {
	return c.params[name]
}

//获取查询参数
func (c *Context) Query(name string) string {
	if c.query == nil {
		c.query = c.Request.URL.Query()
	}
	return c.query.Get(name)
}

func (c *Context) FormValue(name string) string {
	return c.Request.FormValue(name)
}

func (c *Context) FormParams() (url.Values, error) {
	if strings.HasPrefix(c.Request.Header.Get(HeaderContentType), MIMEMultipartForm) {
		if err := c.Request.ParseMultipartForm(defaultMemory); err != nil {
			return nil, err
		}
	} else {
		if err := c.Request.ParseForm(); err != nil {
			return nil, err
		}
	}
	return c.Request.Form, nil
}

func (c *Context) FormFile(name string) (*multipart.FileHeader, error) {
	f, fh, err := c.Request.FormFile(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return fh, nil
}

func (c *Context) MultipartForm() (*multipart.Form, error) {
	err := c.Request.ParseMultipartForm(defaultMemory)
	return c.Request.MultipartForm, err
}

func (c *Context) Cookie(name string) (*http.Cookie, error) {
	return c.Request.Cookie(name)
}

func (c *Context) SetCookie(cookie *http.Cookie) {
	http.SetCookie(c.Response, cookie)
}

func (c *Context) Cookies() []*http.Cookie {
	return c.Request.Cookies()
}

//func (c *Context) Get(key string) interface{} {
//
//}
//
//func (c *Context) Set(key string, val interface{}) {
//
//}

func (c *Context) Bind(i interface{}) error {
	return c.Engine.Binder.Bind(i, c)
}

func (c *Context) Validate(i interface{}) error {
	if c.Engine.Validator == nil {
		return ErrValidatorNotRegistered
	}
	return c.Engine.Validator.Validate(i)
}

func (c *Context) Render(name string, data interface{}) (err error) {
	if c.Engine.Renderer == nil {
		return ErrRendererNotRegistered
	}
	buf := new(bytes.Buffer)
	if err = c.Engine.Renderer.Render(buf, name, data, c); err != nil {
		return
	}
	return c.Blob(MIMETextHTMLCharsetUTF8, buf.Bytes())
}

//结束响应，返回空内容
func (c *Context) End() error {
	c.Response.WriteHeader(0)
	return nil
}

func (c *Context) HTML(html string) (err error) {
	return c.Blob(MIMETextHTMLCharsetUTF8, []byte(html))
}

func (c *Context) String(s string) (err error) {
	return c.Blob(MIMETextPlainCharsetUTF8, []byte(s))
}

func (c *Context) JSON(i interface{}) error {
	data, err := json.Marshal(i)
	if err != nil {
		return err
	}
	return c.Blob(MIMEApplicationJSONCharsetUTF8, data)
}

func (c *Context) JSONP(callback string, i interface{}) (err error) {
	enc := json.NewEncoder(c.Response)
	c.writeContentType(MIMEApplicationJavaScriptCharsetUTF8)
	if _, err = c.Response.Write([]byte(callback + "(")); err != nil {
		return
	}
	if err = enc.Encode(i); err != nil {
		return
	}
	if _, err = c.Response.Write([]byte(");")); err != nil {
		return
	}
	return
}

func (c *Context) XML(i interface{}, indent string) (err error) {
	data, err := xml.Marshal(i)
	if err != nil {
		return err
	}
	c.Blob(MIMEApplicationXMLCharsetUTF8, data)
	return
}

func (c *Context) Blob(contentType string, b []byte) (err error) {
	c.writeContentType(contentType)
	_, err = c.Response.Write(b)
	return
}

func (c *Context) Stream(contentType string, r io.Reader) (err error) {
	c.writeContentType(contentType)
	_, err = io.Copy(c.Response, r)
	return
}

func (c *Context) File(file string) (err error) {
	f, err := os.Open(file)
	if err != nil {
		return MethodNotFoundHandler(c)
	}
	defer f.Close()

	fi, _ := f.Stat()
	if fi.IsDir() {
		file = filepath.Join(file, indexPage)
		f, err = os.Open(file)
		if err != nil {
			return MethodNotFoundHandler(c)
		}
		defer f.Close()
		if fi, err = f.Stat(); err != nil {
			return
		}
	}
	http.ServeContent(c.Response, c.Request, fi.Name(), fi.ModTime(), f)
	return
}

func (c *Context) Inline(file, name string) error {
	return c.contentDisposition(file, name, "inline")
}

func (c *Context) Attachment(file, name string) error {
	return c.contentDisposition(file, name, "attachment")
}

func (c *Context) contentDisposition(file, name, dispositionType string) error {
	c.Response.Header().Set(HeaderContentDisposition, fmt.Sprintf("%s; filename=%q", dispositionType, name))
	return c.File(file)
}

func (c *Context) Redirect(url string) error {
	c.Response.Header().Set(HeaderLocation, url)
	if c.Response.httpStatusCode == 0 {
		c.Response.Status(http.StatusMultipleChoices)
	}
	return nil
}

func (c *Context) Error(err error) {
	c.Engine.HTTPErrorHandler(c, err)
}

func (c *Context) reset(r *http.Request, w http.ResponseWriter) {
	c.index = 0
	c.query = nil
	c.params = nil
	c.matchPath = nil

	c.Path = ""
	c.Routes = nil
	c.Request = r
	c.Response.reset(w)
}