package seed

import _ "embed"

// SQL is deterministic fictional demonstration data for local development.
//
//go:embed seed.sql
var SQL string

//go:embed demo.pdf
var DemoDocument []byte
