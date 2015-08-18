package router

import (
	"mime/multipart"
	"net/http"
	"strings"
)

// Context is a request context wrapping a response writer and the request details
type Context interface {
	// Context acts as a facade on responseWriter
	http.ResponseWriter

	// Request returns the http.Request embedded in this context
	Request() *http.Request

	// Request returns the cleaned path for this request
	Path() string

	// Route returns the route handling for this request
	Route() *Route

	// Config returns a key from the context config
	Config(key string) string

	// Production returns true if we are running in a production environment
	Production() bool

	// Params returns all params for a request
	Params() (Params, error)

	// Param returns a key from the request params
	Param(key string) string

	// ParamInt returns an int64 key from the request params
	ParamInt(key string) int64

	// ParamFiles parses the request as multipart, and then returns the file parts for this key
	ParamFiles(key string) ([]*multipart.FileHeader, error)

	// Store arbitrary data for this request
	Set(key string, data interface{})

	// Retreive arbitrary data for this request
	Get(key string) interface{}

	// Return the rendering context (our data)
	RenderContext() map[string]interface{}

	// Log a message
	Log(message string)

	// Log a format and arguments
	Logf(format string, v ...interface{})
}

// ConcreteContext is the request context, including a writer, the current request etc
type ConcreteContext struct {

	// The current response writer
	writer http.ResponseWriter

	// The current request
	request *http.Request

	// The handling route
	route *Route

	// The parsed and cleaned request path
	path string

	// The context log passed from router
	logger Logger

	// The app config usually loaded from fragmenta.json
	config Config

	// Arbitrary user data stored in a map
	data map[string]interface{}
}

// Request returns the current http Request
func (c *ConcreteContext) Request() *http.Request {
	return c.request
}

// Route returns the route handling this request
func (c *ConcreteContext) Route() *Route {
	return c.route
}

// Header calls our writer and returns the header map that will be sent by WriteHeader.
func (c *ConcreteContext) Header() http.Header {
	return c.writer.Header()
}

// Write calls our writer and writes the data to the connection as part of an HTTP reply.
func (c *ConcreteContext) Write(b []byte) (int, error) {
	return c.writer.Write(b)
}

// WriteHeader calls our writer and sends an HTTP response header with status code.
func (c *ConcreteContext) WriteHeader(i int) {
	c.writer.WriteHeader(i)
}

// Logf logs the given message and arguments using our logger
func (c *ConcreteContext) Logf(format string, v ...interface{}) {
	c.logger.Printf(format, v...)
}

// TODO: Replace usages of Log with Logf, then remove  v ...interface{}

// Log logs the given message using our logger
func (c *ConcreteContext) Log(message string) {
	c.Logf(message)
}

// Params loads and return all the params from the request
func (c *ConcreteContext) Params() (Params, error) {
	params := Params{}

	// Can we somehow parse multipart instead if the request is a multipart request?

	// If we don't have params already, parse the request
	if c.request.Form == nil {
		err := c.parseRequest()
		if err != nil {
			c.Logf("Error parsing request params %s", err)
			return nil, err
		}

	}

	// Add the request form values
	for k, v := range c.request.Form {
		for _, vv := range v {
			params.Add(k, vv)
		}
	}

	// Now add the route params to this list of params
	if c.route.Params == nil {
		c.route.Parse(c.path)
	}
	for k, v := range c.route.Params {
		params.Add(k, v)
	}

	// Return entire params
	return params, nil
}

// Param retreives a single param value, ignoring multiple values
// This may trigger a parse of the request and route
func (c *ConcreteContext) Param(key string) string {

	params, err := c.Params()
	if err != nil {
		c.Logf("Error parsing request %s", err)
		return ""
	}

	return params.Get(key)
}

// ParamInt retreives a single param value as int, ignoring multiple values
// This may trigger a parse of the request and route
func (c *ConcreteContext) ParamInt(key string) int64 {
	params, err := c.Params()
	if err != nil {
		c.Logf("Error parsing request %s", err)
		return 0
	}

	return params.GetInt(key)
}

// ParamFiles parses the request as multipart, and then returns the file parts for this key
// NB it calls ParseMultipartForm prior to reading the parts
func (c *ConcreteContext) ParamFiles(key string) ([]*multipart.FileHeader, error) {
	var parts []*multipart.FileHeader

	err := c.request.ParseMultipartForm(1024 * 20)
	if err != nil {
		return parts, err
	}

	return c.request.MultipartForm.File[key], nil

	/*
		// Research the best approach here
			//get the multipart reader for the request.
			reader, err := c.request.MultipartReader()

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

	*/
}

// Path returns the path for the request
func (c *ConcreteContext) Path() string {
	return c.path
}

// Config returns a key from the context config
func (c *ConcreteContext) Config(key string) string {
	return c.config.Config(key)
}

// Production returns whether this context is running in production
func (c *ConcreteContext) Production() bool {
	return c.config.Production()
}

// Set saves arbitrary data for this request
func (c *ConcreteContext) Set(key string, data interface{}) {
	c.data[key] = data
}

// Get retreives arbitrary data for this request
func (c *ConcreteContext) Get(key string) interface{} {
	return c.data[key]
}

// RedirectStatus redirects setting the status code (for example unauthorized)
// We don't accept external or relative paths for security reasons
func RedirectStatus(context Context, path string, status int) {

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
		http.Redirect(context, context.Request(), path, status)
		return
	}

	context.Logf("#error Ignoring redirect to external path %s", path)
}

// RenderContext returns a context for rendering the view
func (c *ConcreteContext) RenderContext() map[string]interface{} {
	return c.data
}

// parseRequest parses our params from the request form (if any)
func (c *ConcreteContext) parseRequest() error {

	// If we have no request body, return
	if c.request.Body == nil {
		return nil
	}

	// If we have a request body, parse it
	// ParseMultipartForm results in a blank error if not multipart

	err := c.request.ParseForm()
	//   err := c.request.ParseMultipartForm(1024*20)
	if err != nil {
		return err
	}

	return nil
}

// routeParam returns a param from the route - this may return empty string
func (c *ConcreteContext) routeParam(key string) string {

	// If we don't have params already, load them
	if c.route.Params == nil {
		c.route.Parse(c.path)
	}

	return c.route.Params[key]
}
