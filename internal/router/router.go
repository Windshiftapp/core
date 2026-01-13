// Package router provides stdlib-based routing infrastructure with middleware support.
package router

import (
	"net/http"
	"strings"
)

// MiddlewareChain composes middleware functions in order.
// Middleware is applied in the order it appears in the slice.
type MiddlewareChain []func(http.Handler) http.Handler

// Then wraps the given handler with all middleware in the chain.
// The first middleware in the chain is the outermost wrapper.
func (c MiddlewareChain) Then(h http.Handler) http.Handler {
	for i := len(c) - 1; i >= 0; i-- {
		h = c[i](h)
	}
	return h
}

// ThenFunc wraps the given handler function with all middleware in the chain.
func (c MiddlewareChain) ThenFunc(h http.HandlerFunc) http.Handler {
	return c.Then(h)
}

// Append creates a new chain with additional middleware appended.
func (c MiddlewareChain) Append(mw ...func(http.Handler) http.Handler) MiddlewareChain {
	newChain := make(MiddlewareChain, len(c)+len(mw))
	copy(newChain, c)
	copy(newChain[len(c):], mw)
	return newChain
}

// RouteGroup applies middleware to a set of routes with an optional path prefix.
type RouteGroup struct {
	mux        *http.ServeMux
	prefix     string
	middleware MiddlewareChain
}

// NewRouteGroup creates a new route group with the given prefix and middleware.
// The prefix is prepended to all route patterns registered on this group.
// Middleware is applied to all handlers registered on this group.
func NewRouteGroup(mux *http.ServeMux, prefix string, mw ...func(http.Handler) http.Handler) *RouteGroup {
	return &RouteGroup{
		mux:        mux,
		prefix:     prefix,
		middleware: mw,
	}
}

// Handle registers a handler with the group's middleware applied.
// The pattern should include the HTTP method, e.g., "GET /items/{id}".
// The group's prefix is prepended to the path portion of the pattern.
func (g *RouteGroup) Handle(pattern string, h http.HandlerFunc) {
	fullPattern := g.prefixPattern(pattern)
	g.mux.Handle(fullPattern, g.middleware.ThenFunc(h))
}

// HandleH registers an http.Handler with the group's middleware applied.
// Use this for handlers that are already wrapped (e.g., by rate limiters).
func (g *RouteGroup) HandleH(pattern string, h http.Handler) {
	fullPattern := g.prefixPattern(pattern)
	g.mux.Handle(fullPattern, g.middleware.Then(h))
}

// HandleWithMiddleware registers a handler with additional per-route middleware.
// Per-route middleware is applied after the group's middleware.
func (g *RouteGroup) HandleWithMiddleware(pattern string, h http.HandlerFunc, mw ...func(http.Handler) http.Handler) {
	fullPattern := g.prefixPattern(pattern)
	chain := g.middleware.Append(mw...)
	g.mux.Handle(fullPattern, chain.ThenFunc(h))
}

// HandleHandler registers an http.Handler (not HandlerFunc) with the group's middleware.
// Deprecated: Use HandleH instead.
func (g *RouteGroup) HandleHandler(pattern string, h http.Handler) {
	g.HandleH(pattern, h)
}

// Group creates a sub-group with additional prefix and middleware.
// The new group inherits this group's prefix and middleware.
func (g *RouteGroup) Group(prefix string, mw ...func(http.Handler) http.Handler) *RouteGroup {
	return &RouteGroup{
		mux:        g.mux,
		prefix:     g.prefix + prefix,
		middleware: g.middleware.Append(mw...),
	}
}

// prefixPattern adds the group's prefix to a pattern.
// Pattern format: "METHOD /path" -> "METHOD /prefix/path"
func (g *RouteGroup) prefixPattern(pattern string) string {
	if g.prefix == "" {
		return pattern
	}

	// Split method from path
	parts := strings.SplitN(pattern, " ", 2)
	if len(parts) == 2 {
		method := parts[0]
		path := parts[1]
		return method + " " + g.prefix + path
	}

	// No method prefix, just prepend to path
	return g.prefix + pattern
}

// Mux returns the underlying http.ServeMux.
func (g *RouteGroup) Mux() *http.ServeMux {
	return g.mux
}

// Prefix returns the group's path prefix.
func (g *RouteGroup) Prefix() string {
	return g.prefix
}
