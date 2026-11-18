//go:build !darwin && !windows

package main

func primaryMouseDown() bool { return false }
