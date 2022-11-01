package main

import (
	"log"

	"github.com/armadaproject/armada/cmd/testsuite/cmd"
	"github.com/armadaproject/armada/internal/common"
)

// Config is handled by cmd/params.go
func main() {
	common.ConfigureCommandLineLogging()
	root := cmd.RootCmd()
	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
