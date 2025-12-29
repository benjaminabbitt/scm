package backends

import (
	"strings"

	pb "github.com/benjaminabbitt/scm/internal/lm/grpc"
)

// BaseBackend provides common functionality for all AI backends.
// Embed this struct in concrete backend implementations.
type BaseBackend struct {
	name    string
	version string
}

// NewBaseBackend creates a new BaseBackend with the given name and version.
func NewBaseBackend(name, version string) BaseBackend {
	return BaseBackend{name: name, version: version}
}

// Name returns the backend identifier.
func (b *BaseBackend) Name() string {
	return b.name
}

// Version returns the backend version.
func (b *BaseBackend) Version() string {
	return b.version
}

// SupportedModes returns the default supported modes (both interactive and oneshot).
func (b *BaseBackend) SupportedModes() []pb.ExecutionMode {
	return []pb.ExecutionMode{pb.ExecutionMode_INTERACTIVE, pb.ExecutionMode_ONESHOT}
}

// AssembleContext combines fragments into a single context string.
// Fragments are joined with "---" separators.
func (b *BaseBackend) AssembleContext(fragments []*pb.Fragment) string {
	if len(fragments) == 0 {
		return ""
	}

	var parts []string
	for _, f := range fragments {
		if f.Content == "" {
			continue
		}
		parts = append(parts, strings.TrimSpace(f.Content))
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// GetPromptContent extracts the prompt content from a request.
// Returns empty string if no prompt is set.
func (b *BaseBackend) GetPromptContent(req *pb.RunRequest) string {
	if req.Prompt != nil {
		return req.Prompt.Content
	}
	return ""
}
