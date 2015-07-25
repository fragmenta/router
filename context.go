package router

import (
	"io"
	"mime/multipart"
	"net/http"
)

// Interface for a session store (backed by unknown storage)
type SessionStore interface {
	Get(string) string
	Set(string, string)
	Load(request *http.Request) error
	Save(http.ResponseWriter) error
	Clear(http.ResponseWriter)
}

// Interface for a model which knows which pks own it
type OwnedModel interface {
	OwnedBy(int64) bool
}

// Interface for a model which will return role and pk value
type RoleModel interface {
	RoleValue() int64
	PrimaryKeyValue() int64
}

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

func (c *Context) Log(format string, v ...interface{}) {
	// Call our internal logger with these arguments
	c.logger.Printf(format, v...)
}

// Call the route authorisation handler to authorise this route,
// given the user, and (optionally) a model object
func (c *Context) Authorize(o ...OwnedModel) bool {
	if c.Route == nil {
		c.Log("ERROR: Attempt to authorize without route for path %s", c.Path)
		return false
	}

	var m OwnedModel
	if len(o) > 0 {
		m = o[0]
	}

	return c.Route.Authorize(c, m)
}

// Load and return ALL the params from the request
func (c *Context) Params() (Params, error) {
	params := Params{}

	// If we don't have params already, parse the request
	if c.Request.Form == nil {
		err := c.parseRequest()
		if err != nil {
			c.Log("Error parsing request params:", err)
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

// Retrieve a single param value, ignoring multiple values
// This may trigger a parse of the request and route
func (c *Context) Param(key string) string {

	params, err := c.Params()
	if err != nil {
		c.Log("Error parsing request:", err)
		return ""
	}

	return params.Get(key)
}

// Retrieve a single param value as int, ignoring multiple values
// This may trigger a parse of the request and route
func (c *Context) ParamInt(key string) int64 {
	params, err := c.Params()
	if err != nil {
		c.Log("Error parsing request:", err)
		return 0
	}

	return params.GetInt(key)
}

// NB this requires ParseMultipartForm to be called
// HMM not working - ERROR:http: multipart handled by ParseMultipartForm
func (c *Context) ParamFiles(key string) ([]*multipart.Part, error) {

	parts := make([]*multipart.Part, 0)

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

// Used by view to extract relevant objects
func (c *Context) CurrentPath() string {
	return c.Path
}

func (c *Context) CurrentUser() interface{} {
	return c.User
}

func (c *Context) Config(key string) string {
	return c.config.Config(key)
}

func (c *Context) Production() bool {
	return c.config.Production()
}

// Redirect uses status 302 by default - this is not a permanent redirect
// We don't accept external or relative paths for security reasons
func Redirect(context *Context, path string) {

	// Check for redirect in params, if it is valid, use that instead of default path
	redirect := context.Param("redirect")
	if len(redirect) > 0 {
		path = redirect
	}

	// We check this is an internal path - to redirect externally use http.Redirect directly
	if AbsoluteInternalPath(path) {
		// 301 - http.StatusMovedPermanently - permanent redirect
		// 302 - http.StatusFound - tmp redirect
		http.Redirect(context.Writer, context.Request, path, http.StatusFound)
	} else {
		context.Log("#error Ignoring redirect to external path")
	}

}

// Redirect setting the status code (for example unauthorized)
// We don't accept external or relative paths for security reasons
func RedirectStatus(context *Context, path string, status int) {

	// Check for redirect in params, if it is valid, use that instead of default path
	redirect := context.Param("redirect")
	if len(redirect) > 0 {
		path = redirect
	}

	// We check this is an internal path - to redirect externally use http.Redirect directly
	if AbsoluteInternalPath(path) {
		// Status may be 301,302 or 401 access denied etc
		http.Redirect(context.Writer, context.Request, path, status)
	}
}

// Redirect setting the status code (for example unauthorized)
// We don't accept external or relative paths for security reasons
func RedirectExternal(context *Context, path string) {
	// 301 - http.StatusMovedPermanently - permanent redirect
	// 302 - http.StatusFound - tmp redirect
	http.Redirect(context.Writer, context.Request, path, http.StatusFound)

}

// Parse our params from the request form (if any)
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

// Ask for a specific param from the route - this may return empty string
func (c *Context) routeParam(key string) string {

	// If we don't have params already, load them
	if c.Route.Params == nil {
		c.Route.Parse(c.Path)
	}

	return c.Route.Params[key]
}
