package main

// version is overridden at build time:
//   go build -ldflags "-X main.version=1.0.0" .
var version = "dev"
