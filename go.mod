module github.com/valkdb/valk-guard

// 1.25.6 is required by github.com/valkdb/postgresparser (see ../valk-postgres-parser/go.mod).
go 1.25.6

require (
	github.com/spf13/cobra v1.8.1
	github.com/valkdb/postgresparser v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/exp v0.0.0-20240506185415-9bf2ced13842 // indirect
)

replace github.com/valkdb/postgresparser => ../valk-postgres-parser
