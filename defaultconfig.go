package greyproxy

import _ "embed"

// DefaultConfig is the embedded default greyproxy.yml configuration.
// It is used when no -C flag is provided on the command line.
//
//go:embed greyproxy.yml
var DefaultConfig []byte
