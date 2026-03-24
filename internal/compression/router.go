package compression

import (
	"context"
	"fmt"
)

// Router dispatches content to the appropriate compressor based on content type.
// For code and JSON, it uses fast local compression.
// For text/unknown content, it delegates to an LLM-based compressor.
type Router struct {
	// Compressors registered by content type capability.
	compressors []Compressor

	// FallbackCompressor handles content that no specialized compressor can process.
	// Typically an LLM-based compressor.
	FallbackCompressor Compressor
}

// NewRouter creates a router with default compressors.
func NewRouter() *Router {
	return &Router{
		compressors: []Compressor{
			NewCodeCompressor(),
			NewJSONCompressor(),
		},
	}
}

// WithFallback sets the fallback compressor for unhandled content types.
func (r *Router) WithFallback(c Compressor) *Router {
	r.FallbackCompressor = c
	return r
}

// AddCompressor registers an additional compressor.
func (r *Router) AddCompressor(c Compressor) *Router {
	r.compressors = append(r.compressors, c)
	return r
}

// Compress routes content to the appropriate compressor.
func (r *Router) Compress(ctx context.Context, filename string, content string, ratio float64) (Result, error) {
	contentType := DetectContentType(filename, content)

	// Find a compressor that can handle this content type
	for _, c := range r.compressors {
		if c.CanHandle(contentType) {
			return c.Compress(ctx, content, ratio)
		}
	}

	// Fall back to LLM-based compression
	if r.FallbackCompressor != nil {
		return r.FallbackCompressor.Compress(ctx, content, ratio)
	}

	// No compressor available - return original content
	return Result{
		Content:        content,
		OriginalSize:   len(content),
		CompressedSize: len(content),
		Ratio:          1.0,
		CompressedElements: []string{
			fmt.Sprintf("no compressor for %s", contentType),
		},
	}, nil
}

// CompressWithType compresses content with an explicit content type.
func (r *Router) CompressWithType(ctx context.Context, contentType ContentType, content string, ratio float64) (Result, error) {
	for _, c := range r.compressors {
		if c.CanHandle(contentType) {
			return c.Compress(ctx, content, ratio)
		}
	}

	if r.FallbackCompressor != nil {
		return r.FallbackCompressor.Compress(ctx, content, ratio)
	}

	return Result{
		Content:        content,
		OriginalSize:   len(content),
		CompressedSize: len(content),
		Ratio:          1.0,
	}, nil
}

// LLMCompressFunc is the function signature for LLM-based compression.
// The function takes content and returns the compressed content, model ID used, and any error.
type LLMCompressFunc func(ctx context.Context, content string) (compressed string, modelID string, err error)

// LLMCompressor wraps an LLM for text compression.
type LLMCompressor struct {
	// CompressFunc is the actual LLM compression function.
	CompressFunc LLMCompressFunc
}

// NewLLMCompressor creates an LLM-based compressor with the given compression function.
func NewLLMCompressor(fn LLMCompressFunc) *LLMCompressor {
	return &LLMCompressor{
		CompressFunc: fn,
	}
}

// CanHandle returns true for text and unknown content types.
func (c *LLMCompressor) CanHandle(ct ContentType) bool {
	switch ct {
	case ContentTypeMarkdown, ContentTypeText, ContentTypeUnknown, ContentTypeYAML:
		return true
	}
	return false
}

// Compress uses the LLM to compress text content.
func (c *LLMCompressor) Compress(ctx context.Context, content string, ratio float64) (Result, error) {
	if c.CompressFunc == nil {
		return Result{
			Content:        content,
			OriginalSize:   len(content),
			CompressedSize: len(content),
			Ratio:          1.0,
		}, fmt.Errorf("LLM compression function not configured")
	}

	compressed, modelID, err := c.CompressFunc(ctx, content)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Content:        compressed,
		OriginalSize:   len(content),
		CompressedSize: len(compressed),
		Ratio:          float64(len(compressed)) / float64(len(content)),
		ModelID:        modelID,
		PreservedElements: []string{
			"llm-compressed",
		},
	}, nil
}
