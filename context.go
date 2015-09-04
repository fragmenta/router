package router

import (
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

// SessionStore is the interface for a session store (backed by unknown storage)
type SessionStore interface {
	Get(string) string
	Set(string, string)
	Load(request *http.Request) error
	Save(http.ResponseWriter) error
	Clear(http.ResponseWriter)
}

// OwnedModel is the interface for a model which knows who owns it - TODO REMOVE WITH AUTH
type OwnedModel interface {
	OwnedBy(int64) bool
}

// RoleModel is the interface for a model has roles (a user) - TODO REMOVE WITH AUTH
type RoleModel interface {
	RoleValue() int64
	PrimaryKeyValue() int64
}

// TODO convert this to a concrete versino of an interface, and use the interface at all times - no need to tie users to this particular implementation.

// Context is the request context, including a writer, the current request etc
type Context struct {

	// The current response writer
	Writer http.ResponseWriter

	// The current request
	Request *http.Request

	// The request path (cleaned)
	Path string

	// The handling route
	Route *Route

	// The authorisation user
	User RoleModel

	// The session store
	Session SessionStore

	// Errors which occured during routing or rendering
	Errors []error

	// The context log passed from router
	logger Logger

	config Config
}

// Logf logs the given message and arguments using our logger
func (c *Context) Logf(format string, v ...interface{}) {
	c.logger.Printf(format, v...)
}

// TODO: Replace usages of Log with Logf, then remove  v ...interface{}

// Log logs the given message using our logger
func (c *Context) Log(format string, v ...interface{}) {
	c.logger.Printf(format, v...)
}

// TODO: Remove this completely - authorisation should be an app concern, not a framework one

// Authorize calls the route authorisation handler to authorise this route,
// given the user, and (optionally) a model object
func (c *Context) Authorize(o ...OwnedModel) bool {
	if c.Route == nil {
		c.Logf("#error Attempt to authorize without route for path %s", c.Path)
		return false
	}

	var m OwnedModel
	if len(o) > 0 {
		m = o[0]
	}

	return c.Route.Authorize(c, m)
}

// Params loads and return all the params from the request
func (c *Context) Params() (Params, error) {
	params := Params{}

	// If we don't have params already, parse the request
	if c.Request.Form == nil {
		err := c.parseRequest()
		if err != nil {
			c.Log("#error parsing request params:", err)
			return nil, err
		}

	}

	// Add the request form values
	for k, v := range c.Request.Form {
		for _, vv := range v {
			params.Add(k, vv)
		}
	}

	// Now add the route params to this list of params
	if c.Route.Params == nil {
		c.Route.Parse(c.Path)
	}
	for k, v := range c.Route.Params {
		params.Add(k, v)
	}

	// Return entire params
	return params, nil
}

// Param retreives a single param value, ignoring multiple values
// This may trigger a parse of the request and route
func (c *Context) Param(key string) string {

	params, err := c.Params()
	if err != nil {
		c.Logf("#error parsing request:", err)
		return ""
	}

	return params.Get(key)
}

// ParamInt retreives a single param value as int, ignoring multiple values
// This may trigger a parse of the request and route
func (c *Context) ParamInt(key string) int64 {
	params, err := c.Params()
	if err != nil {
		c.Logf("#error parsing request:", err)
		return 0
	}

	return params.GetInt(key)
}

// ParamFiles retreives the files from params
// NB this requires ParseMultipartForm to be called
func (c *Context) ParamFiles(key string) ([]*multipart.Part, error) {

	var parts []*multipart.Part

	//get the multipart reader for the request.
	reader, err := c.Request.MultipartReader()

	if err != nil {
		return parts, err
	}

	//copy each part.
	for {

		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}

		//if part.FileName() is empty, skip this iteration.
		if part.FileName() == "" {
			continue
		}

		parts = append(parts, part)

	}

	return parts, nil
}

// CurrentPath returns the path for the request
func (c *Context) CurrentPath() string {
	return c.Path
}

// CurrentUser returns the user for the request
func (c *Context) CurrentUser() interface{} {
	return c.User
}

// Config returns a key from the context config
func (c *Context) Config(key string) string {
	return c.config.Config(key)
}

// Production returns whether this context is running in production
func (c *Context) Production() bool {
	return c.config.Production()
}

// Redirect uses status 302 StatusFound by default - this is not a permanent redirect
// We don't accept external or relative paths for security reasons
func Redirect(context *Context, path string) {
	// 301 - http.StatusMovedPermanently - permanent redirect
	// 302 - http.StatusFound - tmp redirect
	RedirectStatus(context, path, http.StatusFound)
}

// RedirectStatus redirects setting the status code (for example unauthorized)
// We don't accept external or relative paths for security reasons
func RedirectStatus(context *Context, path string, status int) {

	// Check for redirect in params, if it is valid, use that instead of default path
	// This is potentially surprising behaviour - find where used and REMOVE IT FIXME:URGENT
	redirect := context.Param("redirect")
	if len(redirect) > 0 {
		path = redirect
	}

	// We check this is an internal path - to redirect externally use http.Redirect directly
	if strings.HasPrefix(path, "/") && !strings.Contains(path, ":") {
		// Status may be any value, e.g.
		// 301 - http.StatusMovedPermanently - permanent redirect
		// 302 - http.StatusFound - tmp redirect
		// 401 - Access denied
		context.Logf("#info Redirecting (%d) to path:%s", status, path)
		http.Redirect(context.Writer, context.Request, path, status)
		return
	}

	context.Logf("#error Ignoring redirect to external path %s", path)
}

// RedirectExternal redirects setting the status code (for example unauthorized), but does no checks on the path
// Use with caution.
func RedirectExternal(context *Context, path string) {
	http.Redirect(context.Writer, context.Request, path, http.StatusFound)

}

// parseRequest parses our params from the request form (if any)
func (c *Context) parseRequest() error {

	// If we have no request body, return
	if c.Request.Body == nil {
		return nil
	}

	// If we have a request body, parse it
	// ParseMultipartForm results in a blank error if not multipart

	err := c.Request.ParseForm()
	//   err := c.Request.ParseMultipartForm(1024*20)
	if err != nil {
		return err
	}

	return nil
}

// routeParam returns a param from the route - this may return empty string
func (c *Context) routeParam(key string) string {

	// If we don't have params already, load them
	if c.Route.Params == nil {
		c.Route.Parse(c.Path)
	}

	return c.Route.Params[key]
}
