// Package tools is the platform's global allow-list of side-effecting
// capabilities an agent may invoke. A tool that is not registered here can never
// run, no matter what a scoped credential claims — the registry is the outermost
// gate of the least-privilege model. Tools are always invoked through the tool
// proxy, never directly by the worker.
package tools

import (
	"context"
	"errors"
	"fmt"
)

// ErrInvalidArgs marks arguments that fail a tool's schema check. The tool proxy
// maps it to ERROR_CODE_INVALID_ARGS (distinct from a guard.ErrBlocked policy
// rejection, which maps to POLICY_BLOCKED).
var ErrInvalidArgs = errors.New("invalid tool arguments")

// Tool is a capability the agent can request.
type Tool interface {
	Name() string
	// Validate checks the arguments against the tool's schema and policy guard
	// BEFORE the call is authorized and executed. It returns ErrInvalidArgs for a
	// schema violation, or guard.ErrBlocked for a policy violation (SSRF, path
	// traversal). The tool proxy runs it before recording tool.allowed.
	Validate(args map[string]any) error
	Execute(ctx context.Context, args map[string]any) (map[string]any, error)
}

// Registry maps tool names to implementations.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry builds a registry from the given tools.
func NewRegistry(ts ...Tool) *Registry {
	r := &Registry{tools: make(map[string]Tool, len(ts))}
	for _, t := range ts {
		r.tools[t.Name()] = t
	}
	return r
}

// Get returns a registered tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Has reports whether a tool is registered.
func (r *Registry) Has(name string) bool {
	_, ok := r.tools[name]
	return ok
}

// Names returns the registered tool names.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.tools))
	for n := range r.tools {
		out = append(out, n)
	}
	return out
}

// Echo is the minimal, side-effect-free demo tool. It returns its input
// unchanged and exists to exercise the scoped-credential + tool-proxy path end
// to end. Richer, guarded tools (HTTP fetch with SSRF checks, document read with
// path confinement) are added alongside the policy guard.
type Echo struct{}

// Name implements Tool.
func (Echo) Name() string { return "echo" }

// Validate implements Tool: input must be a present string.
func (Echo) Validate(args map[string]any) error {
	v, ok := args["input"]
	if !ok {
		return fmt.Errorf("%w: echo requires %q", ErrInvalidArgs, "input")
	}
	if _, ok := v.(string); !ok {
		return fmt.Errorf("%w: echo %q must be a string", ErrInvalidArgs, "input")
	}
	return nil
}

// Execute implements Tool.
func (Echo) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	return map[string]any{"echoed": args["input"]}, nil
}
