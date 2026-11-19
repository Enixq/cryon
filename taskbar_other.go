//go:build !windows

package main

import "context"

func startTaskbarControls(context.Context) {}
func stopTaskbarControls()                 {}
func setTaskbarPlaybackState(bool, bool)   {}
