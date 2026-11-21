//go:build !windows

package main

import "log"

// platform refuses the commands only Windows offers
func platform(args []string) {
	log.Fatalf("%q runs on Windows alone", args[0])
}
