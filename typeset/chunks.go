package typeset

import (
	"context"
	"fmt"
	"image"

	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/nttypeset"
)

// ChunkOptions configures lazy extraction. Limits only lower the fixed adapter
// ceilings; zero uses defaults. Options and supplied assets must stay immutable.
type ChunkOptions struct {
	Options Options
	Limits  document.TypesetChunkLimits
}

// ChunkSource implements nttypeset.ChunkSource without retaining output ASTs,
// source maps, or generated assets. The parsed input and a bounded source index
// remain resident. Callers must release returned documents/maps after use.
type ChunkSource struct {
	index *document.TypesetChunks
	opts  Options
}

var _ nttypeset.ChunkSource = (*ChunkSource)(nil)

// BuildChunked indexes semantic HTML units without building or highlighting the
// output AST. The index is fixed for this source's lifetime; rebuild when the
// input, CSS reference width, color scheme, or assets change.
func BuildChunked(ctx context.Context, doc *document.Document, opts ChunkOptions) (*ChunkSource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Snapshot allowlists and cap references as well as decoded pixel residency.
	count, pixels, keyBytes := 0, int64(0), 0
	copyImages := func(in map[string]image.Image) (map[string]image.Image, error) {
		if in == nil {
			return nil, nil
		}
		out := make(map[string]image.Image)
		for key, img := range in {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			count++
			keyBytes += len(key)
			if keyBytes > 1<<20 {
				return nil, fmt.Errorf("%w: chunk asset keys", document.ErrLimit)
			}
			if count > 256 {
				return nil, fmt.Errorf("%w: chunk asset count", document.ErrLimit)
			}
			if img != nil {
				b := img.Bounds()
				w, h := int64(b.Dx()), int64(b.Dy())
				if w < 0 || h < 0 || w > 16<<20 || h > 16<<20 {
					return nil, fmt.Errorf("%w: chunk asset bounds", document.ErrLimit)
				}
				pixels += w * h
			}
			if pixels > 16<<20 {
				return nil, fmt.Errorf("%w: chunk asset pixels", document.ErrLimit)
			}
			out[key] = img
		}
		return out, nil
	}
	var err error
	o := opts.Options
	if o.Document.Images, err = copyImages(o.Document.Images); err != nil {
		return nil, err
	}
	if o.Document.SVGImages, err = copyImages(o.Document.SVGImages); err != nil {
		return nil, err
	}
	// CSS backgrounds are not consumed by this adapter.
	o.Document.BackgroundImages = nil
	idx, err := doc.IndexTypesetChunks(ctx, o.Document, opts.Limits)
	if err != nil {
		return nil, err
	}
	return &ChunkSource{index: idx, opts: o}, nil
}

// Chunks returns the fixed number of chunks, in document order.
func (s *ChunkSource) Chunks() int { return s.index.Chunks() }

// Chunk builds only index. Returned block indexes and positions are chunk-local.
func (s *ChunkSource) Chunk(ctx context.Context, index int) (*nttypeset.Document, error) {
	d, _, err := s.ChunkWithSources(ctx, index)
	return d, err
}

// ChunkWithSources builds a chunk and its local source map. SourceID strings
// retain document-wide identity; BlockRange refers only to this returned AST.
// Reopened ancestor sections retain their IDs and decoration in every chunk.
func (s *ChunkSource) ChunkWithSources(ctx context.Context, index int) (*nttypeset.Document, *SourceMap, error) {
	td, err := s.index.Extract(ctx, index, s.opts.Document)
	if err != nil {
		return nil, nil, err
	}
	pixels := int64(0)
	for _, b := range td.Blocks {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if b.Image != nil {
			r := b.Image.Bounds()
			w, h := int64(r.Dx()), int64(r.Dy())
			if w < 0 || h < 0 || w > 16<<20 || h > 16<<20 {
				return nil, nil, fmt.Errorf("%w: chunk image bounds", document.ErrLimit)
			}
			pixels += w * h
		}
		if pixels > 16<<20 {
			return nil, nil, fmt.Errorf("%w: chunk image pixels", document.ErrLimit)
		}
	}
	return convertDocument(ctx, td, s.opts)
}

// ChunkRange resolves a raw ID or local #fragment to a half-open range of chunk
// indexes. Navigate to start, load that chunk, then use SourceMap.AnchorBounds
// (or Layout.AnchorBounds) for geometry. An ancestor may span several chunks.
// Empty anchors with no rendered content may have no local geometry. Duplicate
// IDs route to their first visible HTML occurrence. "#" routes to chunk zero.
func (s *ChunkSource) ChunkRange(idOrHref string) (start, end int, ok bool) {
	id, valid := normalizeID(idOrHref)
	if !valid {
		return 0, 0, false
	}
	if id == "" {
		return 0, 1, true
	}
	return s.index.ChunkRange(id)
}
