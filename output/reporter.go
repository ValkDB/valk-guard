package output

import (
	"io"

	"github.com/valkdb/valk-guard/rules"
)

// Reporter formats and writes findings to an output destination.
type Reporter interface {
	// Report writes the findings to the given writer.
	Report(w io.Writer, findings []rules.Finding) error
}
