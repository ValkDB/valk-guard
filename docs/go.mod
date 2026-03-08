// This go.mod exists so that the docs/ subtree is a separate Go module.
// Without it, `go install github.com/valkdb/valk-guard/cmd/valk-guard@...`
// would pull all docs, SVGs, and demo media into the module zip.
// With it, the parent module excludes this directory automatically.

module github.com/valkdb/valk-guard/docs

go 1.25.8
