package output

import (
	"context"
	"encoding/json"
	"io"

	"github.com/valkdb/valk-guard/internal/rules"
)

// JSONReporter outputs findings as a JSON array.
type JSONReporter struct{}

// Report writes findings as JSON.
func (r *JSONReporter) Report(ctx context.Context, w io.Writer, findings []rules.Finding) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if findings == nil {
		findings = []rules.Finding{}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(findings)
}
