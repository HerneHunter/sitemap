package sitemap

import (
	"context"
	"io"
)

func parse(ctx context.Context, reader io.Reader, flags parseFlags, useCustomLexer bool, kindOut *bool, yield func(ParseResult) bool) {
	if useCustomLexer {
		parseWithCustomLexer(ctx, reader, flags, kindOut, yield)
	} else {
		parseSafe(ctx, reader, flags, kindOut, yield)
	}
}
