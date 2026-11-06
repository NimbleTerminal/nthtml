package document

import (
	"context"
	"image"
	"math"
	"sort"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

// Painter repeatedly paints one immutable Result. It retains bounded font/face
// caches and an index of display operations. A host must serialize Paint/Close
// calls, keep supplied fonts and assets immutable, and Close when replacing the
// layout. A Painter does not retain previously returned raster images.
type Painter struct {
	result     *Result
	layout     *layout
	buckets    map[int][]int
	broad      []int
	seen       []uint32
	epoch      uint32
	candidates []int
	selected   []operation
	closed     bool
}

const paintBucketHeight = 512
const paintMaxBucketsPerOp = 8

// NewPainter indexes the layout once; it does not paint or allocate a raster.
// Each operation has at most eight bucket references. Tall operations occupy
// one shared list, so large backgrounds cannot multiply index residency.
func (r *Result) NewPainter(ctx context.Context) (*Painter, error) {
	if r == nil || r.document == nil {
		return nil, limitError("nil result")
	}
	p := &Painter{result: r, buckets: make(map[int][]int), seen: make([]uint32, len(r.ops))}
	p.layout = &layout{d: r.document, opts: r.opts, lim: r.limits, canvasBg: r.Background(), maxHeight: max(r.size.Y, r.document.limits.Height), fonts: make(map[string]*opentype.Font)}
	for i, op := range r.ops {
		if err := ctx.Err(); err != nil {
			p.Close()
			return nil, err
		}
		bounds := op.box.Bounds.Union(op.box.InkBounds).Intersect(r.Bounds())
		if !op.box.Clip.Empty() {
			bounds = bounds.Intersect(op.box.Clip)
		}
		if bounds.Empty() {
			continue
		}
		first, last := bounds.Min.Y/paintBucketHeight, (bounds.Max.Y-1)/paintBucketHeight
		if last-first+1 > paintMaxBucketsPerOp {
			p.broad = append(p.broad, i)
			continue
		}
		for bucket := first; bucket <= last; bucket++ {
			p.buckets[bucket] = append(p.buckets[bucket], i)
		}
	}
	return p, nil
}

func (r *Result) paintSize(ctx context.Context, area image.Rectangle, scale float64) (image.Rectangle, image.Point, error) {
	if r == nil || r.document == nil {
		return image.Rectangle{}, image.Point{}, limitError("nil result")
	}
	if err := ctx.Err(); err != nil {
		return image.Rectangle{}, image.Point{}, err
	}
	area = area.Intersect(r.Bounds())
	if area.Empty() {
		return area, image.Point{}, limitError("paint area outside the page")
	}
	if !(scale >= 1.0/16 && scale <= 16) {
		return area, image.Point{}, limitError("scale outside 1/16..16")
	}
	size := image.Pt(int(math.Ceil(float64(area.Dx())*scale)), int(math.Ceil(float64(area.Dy())*scale)))
	if size.X < 1 || size.Y < 1 || size.X > r.PixelLimit()/size.Y {
		return area, size, limitError("pixels")
	}
	return area, size, nil
}

// PixelLimit is the maximum raster allocation in pixels for this layout.
func (r *Result) PixelLimit() int {
	if r == nil {
		return 0
	}
	if r.limits.Pixels > 0 {
		return r.limits.Pixels
	}
	if r.document != nil {
		return r.document.limits.Pixels
	}
	return DefaultLimits.Pixels
}

func (p *Painter) clearFaces() {
	l := p.layout
	for _, f := range l.faces {
		f.Close()
	}
	l.faces = make(map[faceKey]font.Face)
	l.primary = nil
	l.faceInfo = nil
	l.textFallbacks = nil
	l.coverage = nil
	l.splits = nil
	l.splitBytes = 0
	l.fontStopped = false
}

// Paint has Result.Paint's coordinates, scale range and pixel budget, while
// reusing font data and examining only indexed candidates (in paint order).
// Changing scale discards raster-size-specific faces but retains parsed fonts.
func (p *Painter) Paint(ctx context.Context, area image.Rectangle, scale float64) (*image.RGBA, error) {
	if p == nil || p.closed {
		return nil, limitError("closed painter")
	}
	area, size, err := p.result.paintSize(ctx, area, scale)
	if err != nil {
		return nil, err
	}
	l := p.layout
	if l.scale != scale {
		p.clearFaces()
		l.scale = scale
	}
	l.ctx, l.err = ctx, nil
	p.epoch++
	if p.epoch == 0 {
		clear(p.seen)
		p.epoch = 1
	}
	p.candidates = p.candidates[:0]
	add := func(ids []int) {
		for _, i := range ids {
			if p.seen[i] != p.epoch {
				p.seen[i] = p.epoch
				p.candidates = append(p.candidates, i)
			}
		}
	}
	add(p.broad)
	for bucket := area.Min.Y / paintBucketHeight; bucket <= (area.Max.Y-1)/paintBucketHeight; bucket++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		add(p.buckets[bucket])
	}
	sort.Ints(p.candidates)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.selected = p.selected[:0]
	for _, i := range p.candidates {
		p.selected = append(p.selected, p.result.ops[i])
	}
	img := image.NewRGBA(image.Rectangle{Max: size})
	if err := l.paint(img, p.selected, area, scale); err != nil {
		p.clearFaces()
		l.fonts = make(map[string]*opentype.Font)
		l.fontBytes = 0
		return nil, err
	}
	return img, nil
}

// Close releases the caches and references to the layout and its assets.
func (p *Painter) Close() {
	if p == nil || p.closed {
		return
	}
	p.clearFaces()
	p.closed = true
	p.result, p.layout = nil, nil
	p.buckets, p.broad, p.seen, p.candidates, p.selected = nil, nil, nil, nil, nil
}
