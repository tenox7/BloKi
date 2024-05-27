//go:build !plan9
// +build !plan9

package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func gitInit() error {
	if !*useGit {
		return nil
	}
	_, err := git.PlainInit(*rootDir, false)
	if err != nil {
		return err
	}
	log.Printf("Git Init %q", *rootDir)
	return nil
}

func gitAdd(file, user string) error {
	if !*useGit {
		return nil
	}
	gr, err := git.PlainOpen(*rootDir)
	if err != nil {
		return fmt.Errorf("unable to open git repo: %v", err)
	}
	wt, err := gr.Worktree()
	if err != nil {
		return fmt.Errorf("unable to get git work tree: %v", err)
	}
	_, err = wt.Add(file)
	if err != nil {
		return fmt.Errorf("unable to add git file %v: %v", file, err)
	}
	hash, err := wt.Commit("User "+user+" modified file "+file, &git.CommitOptions{
		Author: &object.Signature{
			Name: user,
			When: time.Now(),
		}})
	if err != nil {
		return fmt.Errorf("unable to commit git: %v", err)
	}
	log.Printf("Git Add: user=%v file=%v commit=%v", user, file, hash)
	return nil
}

func gitDelete(file, user string) error {
	if !*useGit {
		log.Printf("User %v deleted %v", user, file)
		return os.Remove(path.Join(*rootDir, file))
	}
	gr, err := git.PlainOpen(*rootDir)
	if err != nil {
		return fmt.Errorf("unable to open git repo: %v", err)
	}
	wt, err := gr.Worktree()
	if err != nil {
		return fmt.Errorf("unable to get git work tree: %v", err)
	}
	_, err = wt.Remove(file)
	if err != nil {
		return fmt.Errorf("unable to delete git file %v: %v", file, err)
	}
	hash, err := wt.Commit("User "+user+" deleted file "+file, &git.CommitOptions{
		Author: &object.Signature{
			Name: user,
			When: time.Now(),
		}})
	if err != nil {
		return fmt.Errorf("unable to commit git: %v", err)

	}
	log.Printf("Git Delete: user=%v file=%v commit=%v", user, file, hash)
	return nil
}

func gitMove(old, new, user string) error {
	if !*useGit {
		log.Printf("User %v renamed %v to %v", user, old, new)
		return os.Rename(path.Join(*rootDir, old), path.Join(*rootDir, new))
	}
	gr, err := git.PlainOpen(*rootDir)
	if err != nil {
		return fmt.Errorf("unable to open git repo: %v", err)
	}
	wt, err := gr.Worktree()
	if err != nil {
		return fmt.Errorf("unable to get git work tree: %v", err)
	}
	_, err = wt.Move(old, new)
	if err != nil {
		return fmt.Errorf("unable to move  %v -> %v: %v", old, new, err)
	}
	hash, err := wt.Commit("User "+user+" renamed file "+old+" to "+new, &git.CommitOptions{
		Author: &object.Signature{
			Name: user,
			When: time.Now(),
		}})
	if err != nil {
		return fmt.Errorf("unable to commit git: %v", err)

	}
	log.Printf("Git Rename: user=%v old=%v new=%v commit=%v", user, old, new, hash)
	return nil
}
