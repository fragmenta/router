// Package router provides a router linking uris to handlers taking a context
package router

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

// ContextHandler handles
type ContextHandler func(Context)

// FilterHandler should be removed as unused at present
type FilterHandler func(Context) error

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

	// File handler
	FileHandler ContextHandler

	// The logger passed to actions within the context on each request
	Logger Logger

	// The server config passed to actions within the context on each request
	Config Config

	// A list of routes
	routes []*Route

	// A list of pre-action filters
	filters []FilterHandler
}

// New creates a new router
func New(l Logger, s Config) (*Router, error) {
	r := &Router{
		FileHandler: fileHandler,
		Logger:      l,
		Config:      s,
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
func (r *Router) Add(pattern string, handler ContextHandler) *Route {
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

// AddFilter adds a new filter
func (r *Router) AddFilter(filter FilterHandler) {
	// Store this filter in the router list
	r.filters = append(r.filters, filter)

}

// Reset stored state in routes (parsed params)
func (r *Router) Reset() {
	for _, r := range r.routes {
		r.Reset()
	}
}

// ServeHTTP - Central dispatcher for web requests - sets up the context and hands off to handlers
func (r *Router) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	// Reset any cached state at the end of each request
	defer r.Reset()

	// Started GET "/" for 127.0.0.1 at 2014-07-01 14:15:32 +0100
	started := time.Now()
	summary := fmt.Sprintf("%s %s for %s", request.Method, request.URL.Path, request.RemoteAddr)

	// Clean the path
	canonicalPath := path.Clean(request.URL.Path)
	if len(canonicalPath) == 0 {
		canonicalPath = "/"
	} else if canonicalPath[0] != '/' {
		canonicalPath = "/" + canonicalPath
	}

	status := 200

	// Log starting the request - we should have some way of excluding logs
	// not hard-coded like this FIXME
	logging := !strings.HasPrefix(canonicalPath, "/assets") && !strings.HasPrefix(canonicalPath, "/files")
	if logging {
		r.Logf("#info Started %s", summary)
	}

	// Set up a handler to handle request if not redirected
	var handler ContextHandler

	var route *Route

	// Try finding a route
	route = r.findRoute(canonicalPath, request)

	if route != nil {

		if logging {
			r.Logf("#info Handling with route %s", route)
		}

		if route.Handler != nil {
			handler = route.Handler
		} else if route.RedirectStatus != 0 {
			// Redirect to RedirectPath and return
			http.Redirect(writer, request, route.RedirectPath, route.RedirectStatus)
			return
		}
	}

	// Setup the context
	context := &ConcreteContext{
		writer:  writer,
		request: request,
		path:    canonicalPath,
		route:   route,
		logger:  r.Logger,
		config:  r.Config,
	}

	// Call any filters
	for _, f := range r.filters {
		err := f(context)
		if err != nil {
			end := time.Since(started).String()
			r.Logf("#error Filter error at %s in %s ERROR:%s", summary, err, end)
			return
		}
	}

	// If handler is not nil, serve, else fall back to defaults
	if handler != nil {

		// Handle the request
		handler(context)

		// Log the end of handling
		end := time.Since(started).String()

		if logging {
			r.Logf("#info Finished %s status %d in %s", summary, status, end)
		}

	} else {
		// If no route or handler, try default file handler to serve static files
		r.FileHandler(context)
	}

}

// This may return nil
// Canonical path should have been cleaned first
func (r *Router) findRoute(canonicalPath string, request *http.Request) *Route {
	for _, r := range r.routes {
		// Check method (GET/PUT), then check path
		if r.MatchMethod(request.Method) && r.MatchPath(canonicalPath) {
			return r
		}
	}
	return nil
}

// Default static file handler - this is the last line of handlers
func fileHandler(context Context) {

	// Assuming we're running from the root of the website
	localPath := "./public" + path.Clean(context.Path)

	if _, err := os.Stat(localPath); err != nil {
		if os.IsNotExist(err) {
			// Where it doesn't exist render not found
			http.Redirect(context.Writer, context.Request, context.Path, http.StatusNotFound)
			return
		}

		// For other errors return not authorised
		http.Redirect(context.Writer, context.Request, context.Path, http.StatusUnauthorized)
		return
	}


	// If the file exists and we can access it, serve it
	http.ServeFile(context.Writer, context.Request, localPath)
}
