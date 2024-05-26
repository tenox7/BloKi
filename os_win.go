//go:build windows
// +build windows

package main

func userId(_ string) (int, int, error) { return 0, 0, nil }
func getSuidSgid() (int, int)           { return 0, 0 }
func setUidGid(_, _ int)                { return }
func chRoot()                           { return }
