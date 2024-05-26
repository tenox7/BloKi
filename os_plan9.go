//go:build plan9
// +build plan9

package main

func userId(_ string) (int, int, error) { return 0, 0, nil }
func getSuidSgid() (int, int)           { return 0, 0 }
func setUidGid(_, _ int)                { return }
func chRoot()                           { return }

type textSearch struct{}

func (t *textSearch) delete(_ string)          {}
func (t *textSearch) rename(_, _ string)       {}
func (t *textSearch) update(file string)       {}
func (t *textSearch) search(_ string) []string { return nil }
func (t *textSearch) rescan()                  {}

func gitInit()                 {}
func gitCommit(_, _, _ string) {}
