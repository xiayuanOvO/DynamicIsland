//go:build !windows && !darwin

package main

func foregroundFullscreen() bool { return false }
