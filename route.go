package router

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// FIXME - remove AuthorizationHandler - move that to app, it should not concern us

// Route stores information to match a request and build URLs.
type Route struct {
	// An HTTP handler which accepts a context
	Handler ContextHandler

	// An authorisation handler
	AuthHandler AuthorizationHandler

	// If the route is simply a string we match against that
	Pattern string

	// Up to three letters to match (before any regexp) for fast decisions on matches
	PatternShort string

	// If the route is a regexp, we match that instead (this may have groups etc)
	Regexp *regexp.Regexp

	// Param names taken from the Pattern and matching params
	ParamNames []string

	// Params taken from the request path parsed with Regexp
	Params map[string]string

	// Redirect path - used to redirect if handler is nil
	RedirectPath string

	// Redirect status - used to redirect if handler is nil
	RedirectStatus int

	// Permitted HTTP methods (GET, POST) - default GET
	methods []string
}

// NewRoute creates a new Route, given a pattern to match and a handler for the route
func NewRoute(pattern string, handler ContextHandler, authHandler AuthorizationHandler) (*Route, error) {

	r := &Route{
		Handler:      handler,
		AuthHandler:  authHandler,
		Pattern:      pattern,
		PatternShort: shortPattern(pattern),
		Regexp:       nil,
		Params:       nil,
		methods:      []string{"GET"}, // NB Get by default
	}

	// Check for regexps within pattern and parse if necessary
	// This can significantly slow down server startup if you had thousands of routes
	// We could instead consider compiling on demand when the match is requested
	if strings.Contains(r.Pattern, "{") {
		err := r.compileRegexp()
		if err != nil {
			return nil, err
		}
	}

	return r, nil
}

// Authorize calls the route authorisation handler to authorize this route,
// given the user, and (optionally) a model object
func (r *Route) Authorize(c *Context, m OwnedModel) bool {

	// Our handler itself must not be nil
	if r.AuthHandler == nil {
		return false
	}

	return r.AuthHandler(c, m)
}

// Get sets the method exclusively to GET
func (r *Route) Get() *Route {
	return r.Method("GET")
}

// Post sets the method exclusively to POST
func (r *Route) Post() *Route {
	return r.Method("POST")
}

// Put sets the method exclusively to PUT
func (r *Route) Put() *Route {
	return r.Method("PUT")
}

// Delete sets the method exclusively to DELETE
func (r *Route) Delete() *Route {
	return r.Method("DELETE")
}

// Method sets the method exclusively to method
func (r *Route) Method(method string) *Route {
	r.methods = []string{method}
	return r
}

// Accept allows the method provided
func (r *Route) Accept(method string) *Route {
	if !r.MatchMethod(method) {
		r.methods = append(r.methods, method)
	}
	return r
}

// Methods sets the methods allowed as an array
func (r *Route) Methods(permitted []string) *Route {
	r.methods = permitted
	return r
}

// Parse reads our params using the regexp from the given path
func (r *Route) Parse(path string) {

	// Set up our params map
	r.Params = make(map[string]string)

	// Go no farther if we have no regexp to match against
	if r.Regexp == nil {
		return
	}

	matches := r.Regexp.FindStringSubmatch(path)

	if matches != nil {
		for i, key := range r.ParamNames {
			index := i + 1
			if len(matches) > index {
				value := matches[index]
				r.Params[key] = value
			}

		}
	}
}

// Auth sets the Authorisation handler
func (r *Route) Auth(handler AuthorizationHandler) *Route {
	r.AuthHandler = handler
	return r
}

// Reset stored state in routes (parsed params)
func (r *Route) Reset() {
	r.Params = nil
}

// MatchMethod returns true if our list of methods contains method
func (r *Route) MatchMethod(method string) bool {

	// We treat "" as GET by default
	if method == "" {
		method = "GET"
	}

	for _, v := range r.methods {
		if v == method {
			return true
		}
	}

	return false
}

// MatchPath returns true if this route matches the path
func (r *Route) MatchPath(path string) bool {

	// Reject asset paths, which we don't handle (server should be handling)
	if strings.HasPrefix(path, "/assets") {
		return false
	}

	// Check against short pattern first, to reject obvious misses
	if len(r.PatternShort) > 0 {
		if !strings.HasPrefix(path, r.PatternShort) {
			return false
		}
	}

	// If we have a short pattern match, and we have a regexp, check against that
	if r.Regexp != nil {
		if r.Regexp.MatchString(path) {
			return true
		}

		// If we don't have regexp, check for a simple string match
	} else if r.Pattern == path {
		return true
	}

	// No match return nil
	return false

}

// compileRegexp compiles our route format to a true regexp
// Both name and regexp are required - routes should be well structured and restrictive by default
// Convert the pattern from the form  /pages/{id:[0-9]*}/edit?param=test
// to one suitable for regexp -  /pages/([0-9]*)/edit\?param=test
// We want to match things like this:
// /pages/{id:[0-9]*}/edit
// /pages/{id:[0-9]*}/edit?param=test
func (r *Route) compileRegexp() (err error) {
	// Check if it is well-formed.
	idxs, errBraces := r.findBraces(r.Pattern)
	if errBraces != nil {
		return errBraces
	}

	pattern := bytes.NewBufferString("^")
	end := 0

	// Walk through indexes two at a time
	for i := 0; i < len(idxs); i += 2 {
		// Set all values we are interested in.
		raw := r.Pattern[end:idxs[i]]
		end = idxs[i+1]
		parts := strings.SplitN(r.Pattern[idxs[i]+1:end-1], ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("Missing name or pattern in %s", raw)
		}

		// Add the Argument name
		r.ParamNames = append(r.ParamNames, parts[0])

		// Add the real regexp
		fmt.Fprintf(pattern, "%s(%s)", regexp.QuoteMeta(raw), parts[1])

	}
	// Add the remaining pattern
	pattern.WriteString(regexp.QuoteMeta(r.Pattern[end:]))
	r.Regexp, err = regexp.Compile(pattern.String())

	return err
}

// findBraces returns the first level curly brace indices from a string.
// It returns an error in case of unbalanced braces.
// This method based on gorilla mux
func (r *Route) findBraces(s string) ([]int, error) {
	var level, idx int
	var idxs []int
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			if level++; level == 1 {
				idx = i
			}
		case '}':
			if level--; level == 0 {
				idxs = append(idxs, idx, i+1)
			} else if level < 0 {
				return nil, fmt.Errorf("Route error: unbalanced braces in %q", s)
			}
		}
	}
	if level != 0 {
		return nil, fmt.Errorf("Route error: unbalanced braces in %q", s)
	}
	return idxs, nil
}

// shortPattern returns at most 3 chars of the pattern before the first {
func shortPattern(p string) string {
	l := 3
	if len(p) < 3 {
		l = len(p)
	}

	// check index of {
	i := strings.Index(p, "{")
	if i > -1 && i < 3 {
		l = i
	}

	return p[:l]
}

// String returns the route formatted as a string
func (r *Route) String() string {
	return fmt.Sprintf("%s %s", r.methods, r.Pattern)
}
