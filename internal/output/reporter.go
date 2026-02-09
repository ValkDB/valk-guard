package output

import (
	"context"
	"io"

	"github.com/valkdb/valk-guard/internal/rules"
)

// Reporter formats and writes findings to an output destination.
type Reporter interface {
	// Report writes the findings to the given writer.
	Report(ctx context.Context, w io.Writer, findings []rules.Finding) error
}
