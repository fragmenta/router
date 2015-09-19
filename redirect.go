// Package router provides a router linking uris to handlers taking a context
package router

import (
	"fmt"
	"net/http"
	"strings"
)

// Redirect uses status 302 StatusFound by default - this is not a permanent redirect
// We don't accept external or relative paths for security reasons
func Redirect(context Context, path string) error {
	// 301 - http.StatusMovedPermanently - permanent redirect
	// 302 - http.StatusFound - tmp redirect
	return RedirectStatus(context, path, http.StatusFound)
}

// RedirectStatus redirects setting the status code (for example unauthorized)
// We don't accept external or relative paths for security reasons
func RedirectStatus(context Context, path string, status int) error {

	// We check this is an internal path - to redirect externally use http.Redirect directly
	if strings.HasPrefix(path, "/") && !strings.Contains(path, ":") {
		// Status may be any value, e.g.
		// 301 - http.StatusMovedPermanently - permanent redirect
		// 302 - http.StatusFound - tmp redirect
		// 401 - Access denied
		context.Logf("#info Redirecting (%d) to path:%s", status, path)
		http.Redirect(context, context.Request(), path, status)
		return nil
	}

	return fmt.Errorf("Ignoring redirect to external path %s", path)
}

// RedirectExternal redirects setting the status code (for example unauthorized), but does no checks on the path
// Use with caution and only on completely known paths.
func RedirectExternal(context Context, path string) error {
	http.Redirect(context, context.Request(), path, http.StatusFound)
	return nil
}
