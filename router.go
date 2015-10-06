// Package router provides a router linking uris to handlers taking a context
package router

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

// Handler is our standard handler function, accepting a router.Context interface, and returning router.Error
type Handler func(Context) error

// ErrHandler is used to render a router.Error - used by ErrorHandler on the router
type ErrHandler func(Context, error)

// Logger Interface for a simple logger (the stdlib log pkg and the fragmenta log pkg conform)
type Logger interface {
	Printf(format string, args ...interface{})
}

// Config Interface to retreive configuration details of the server
type Config interface {
	Production() bool
	Config(string) string
}

// Router stores and handles the routes
type Router struct {
	// Mutex protects routes and filters
	mu sync.RWMutex

	// File handler (sends files)
	FileHandler Handler

	// Error handler (renders errors)
	ErrorHandler ErrHandler

	// The logger passed to actions within the context on each request
	Logger Logger

	// The server config passed to actions within the context on each request
	Config Config

	// A list of routes
	routes []*Route

	// A list of pre-action filters, applied before any handler
	filters []Handler
}

// New creates a new router
func New(l Logger, s Config) (*Router, error) {
	r := &Router{
		FileHandler:  fileHandler,
		ErrorHandler: errHandler,
		Logger:       l,
		Config:       s,
	}

	// Set our router to handle all routes
	http.Handle("/", r)
	return r, nil
}

// Logf logs this message and the given arguments
func (r *Router) Logf(format string, v ...interface{}) {
	r.Logger.Printf(format, v...)
}

// Log this format and arguments
func (r *Router) Log(message string) {
	r.Logf(message)
}

// Add a new route
func (r *Router) Add(pattern string, handler Handler) *Route {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Create a new route
	route, err := NewRoute(pattern, handler)
	if err != nil {
		r.Logf("#error Creating regexp failed for route %s:%s", pattern, err)
	}

	// Store this route in the router
	r.routes = append(r.routes, route)

	// Return route for chaining
	return route
}

// AddRedirect adds a new redirect this is just a route with a redirect path set
func (r *Router) AddRedirect(pattern string, redirectPath string, status int) *Route {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Create a new route for redirecting - NB no handler or auth handler
	route, err := NewRoute(pattern, nil)
	if err != nil {
		r.Logf("#error Creating redirect failed for route %s:%s", pattern, err)
	}
	route.RedirectPath = redirectPath
	route.RedirectStatus = status

	// Store this route in the router
	r.routes = append(r.routes, route)

	// Return route for chaining
	return route
}

// AddFilter adds a new filter to our list of filters to execute before request handlers
func (r *Router) AddFilter(filter Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Store this filter in the router list
	r.filters = append(r.filters, filter)

}

// AddFilterHandler adds a standard http.Handler to filters wrapped in a ContextHandler
func (r *Router) AddFilterHandler(handler http.Handler) {
	f := func(context Context) error {
		handler.ServeHTTP(context.Writer(), context.Request())
		return nil
	}
	r.AddFilter(f)
}

// AddFilterHandlerFunc adds a standard http.HandlerFunc to filters wrapped in a ContextHandler
func (r *Router) AddFilterHandlerFunc(handler http.HandlerFunc) {
	f := func(context Context) error {
		handler(context.Writer(), context.Request())
		return nil
	}
	r.AddFilter(f)
}

// ServeHTTP - Central dispatcher for web requests - sets up the context and hands off to handlers
func (r *Router) ServeHTTP(writer http.ResponseWriter, request *http.Request) {

	// Lock handlers/filters for duration of handling
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Started GET "/" for 127.0.0.1 at 2014-07-01 14:15:32 +0100
	started := time.Now()
	summary := fmt.Sprintf("%s %s for %s", request.Method, request.URL.Path, remoteIP(request))

	// Clean the path
	canonicalPath := path.Clean(request.URL.Path)
	if len(canonicalPath) == 0 {
		canonicalPath = "/"
	} else if canonicalPath[0] != '/' {
		canonicalPath = "/" + canonicalPath
	}

	status := 200

	// Log starting the request
	// FIXME: We should have some way of excluding logs not hard-coded like this
	logging := !strings.HasPrefix(canonicalPath, "/assets") && !strings.HasPrefix(canonicalPath, "/files")
	if logging {
		r.Logf("#info Started %s", summary)
	}

	// Try finding a route
	route := r.findRoute(canonicalPath, request)

	// Our handler may end as nil
	var handler Handler
	if route != nil {

		// Log route info
		if logging {
			r.Logf("#info Handling with route %s", route)
		}

		// Handle redirects by redirecting and doing no more
		if route.RedirectStatus != 0 {
			http.Redirect(writer, request, route.RedirectPath, route.RedirectStatus)
			return
		}

		handler = route.Handler
	}

	// Setup the context
	context := &ConcreteContext{
		writer:  writer,
		request: request,
		path:    canonicalPath,
		route:   route,
		logger:  r.Logger,
		config:  r.Config,
		data:    make(map[string]interface{}, 0),
	}

	// Call any filters
	for _, f := range r.filters {
		err := f(context)
		if err != nil {
			r.ErrorHandler(context, err)
			return
		}
	}

	// If handler is not nil, serve, else fall back to defaults
	if handler != nil {

		// Handle the request
		err := handler(context)
		if err != nil {
			r.ErrorHandler(context, err)
			return
		}

		// Log the end of handling
		end := time.Since(started).String()

		if logging {
			r.Logf("#info Finished %s status %d in %s", summary, status, end)
		}

	} else {
		// If no route or handler, try default file handler to serve static files (no logging)
		err := r.FileHandler(context)
		if err != nil {
			r.ErrorHandler(context, err)
			return
		}
	}

}

// findRoute finds the matching route given a cleaned path - this may return nil
func (r *Router) findRoute(canonicalPath string, request *http.Request) *Route {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, r := range r.routes {
		// Check method (GET/PUT), then check path
		if r.MatchMethod(request.Method) && r.MatchPath(canonicalPath) {
			return r
		}
	}
	return nil
}

// fileHandler is the default static file handler - this is the last line of handlers
func fileHandler(context Context) error {

	// Assuming we're running from the root of the website
	localPath := "./public" + path.Clean(context.Path())

	if _, err := os.Stat(localPath); err != nil {
		if os.IsNotExist(err) {
			// Where it doesn't exist render not found
			//	http.Redirect(context, context.Request(), context.Path(), http.StatusNotFound)
			return NotFoundError(err)
		}

		// For other errors return not authorised
		//	http.Redirect(context, context.Request(), context.Path(), http.StatusUnauthorized)
		return NotAuthorizedError(err)
	}

	// If the file exists and we can access it, serve it
	http.ServeFile(context.Writer(), context.Request(), localPath)

	return nil
}

// errHandler is a simple error handler which writes the error to context.Writer
func errHandler(context Context, e error) {

	// Cast the error to a status error if it is one, if not wrap it in a Status 500 error
	err := ToStatusError(e)

	// Get the writer from context and write the error page
	writer := context.Writer()

	// Set the headers
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(err.Status)

	// Write a simple error message page
	html := fmt.Sprintf("<h1>%s</h1><p>%s</p>", err.Title, err.Message)

	// If NOT in production, write a more complex page which reveals the real error (later stack trace etc)
	if !context.Production() {
		html = fmt.Sprintf("<h1>%s</h1><p>%s</p><p>Error %d at %s</p><p><code>Error:%s</code></p>",
			err.Title, err.Message, err.Status, err.FileLine(), err.Err.Error())
	}

	context.Logf("#error %s\n", err)
	io.WriteString(writer, html)
}

func remoteIP(request *http.Request) string {
	address := request.Header.Get("X-Real-IP")
	if len(address) > 0 {
		return address
	}

	address = request.Header.Get("X-Forwarded-For")
	if len(address) > 0 {
		return address
	}

	return request.RemoteAddr
}
