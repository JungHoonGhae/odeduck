package dataset

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
)

// Shared bounded download of an already inspected direct asset. Never accepts
// a caller-built URL or archive path as a new external request.
func (i *Inspector) downloadSampleAsset(ctx context.Context, asset Asset, format string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !strings.EqualFold(path.Ext(asset.Name), "."+format) && !strings.EqualFold(asset.Format, format) {
		return nil, fmt.Errorf("%s sampling requires an inspected direct %s asset", format, format)
	}
	r, err := i.openAsset(ctx, asset.Request)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	if r.Status != http.StatusOK {
		return nil, fmt.Errorf("%s sample download status %d", format, r.Status)
	}
	const maxBytes = 8 << 20
	if r.ContentLength > maxBytes {
		return nil, fmt.Errorf("%s sample download exceeds 8 MiB", format)
	}
	body, err := io.ReadAll(io.LimitReader(sampleContextReader{ctx: ctx, r: r.Body}, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBytes {
		return nil, fmt.Errorf("%s sample download exceeds 8 MiB", format)
	}
	return body, ctx.Err()
}

type sampleContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r sampleContextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(b)
}
