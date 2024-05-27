//go:build plan9
// +build plan9

package main

import (
	"os"
	"path"
	"time"
)

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

func gitInit() error           { return nil }
func gitCommit(_, _, _ string) {}
func gitAdd(_, _ string) error { return nil }
func gitDelete(file, _ string) error {
	return os.Remove(path.Join(*rootDir, file))
}
func gitMove(old, new, _ string) error {
	return os.Rename(path.Join(*rootDir, old), path.Join(*rootDir, new))
}

type commitList struct {
	author  string
	time    time.Time
	message string
}

func gitList() ([]commitList, error) { return nil, nil }
