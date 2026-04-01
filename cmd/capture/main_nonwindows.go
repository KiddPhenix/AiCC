//go:build !windows

package main

import "log"

func main() {
	log.Fatal("capture app currently supports Windows only")
}
