//go:build !windows
// +build !windows

package main

import (
	"log"
	"os"
	"os/user"
	"regexp"
	"strconv"
	"syscall"
)

func userId(usr string) (int, int, error) {
	u, err := user.Lookup(usr)
	if err != nil {
		return 0, 0, err
	}
	ui, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, err
	}
	gi, err := strconv.Atoi(u.Gid)
	if err != nil {
		return 0, 0, err
	}
	return ui, gi, nil
}

func getSuidSgid() (int, int) {
	if *suidUser == "" {
		return 0, 0
	}
	var suid, sgid int
	var err error

	uidSm := regexp.MustCompile(`^(\d+):(\d+)$`).FindStringSubmatch(*suidUser)
	switch len(uidSm) {
	case 3:
		suid = atoiOrFatal(uidSm[1])
		sgid = atoiOrFatal(uidSm[2])
	default:
		suid, sgid, err = userId(*suidUser)
		if err != nil {
			log.Fatal("unable to find setuid user", err)
		}
	}
	log.Printf("Found IDs for %q: suid=%v sgid=%v", *suidUser, suid, sgid)
	return suid, sgid
}

func setUidGid(ui, gi int) {
	if ui == 0 || gi == 0 {
		return
	}
	err := syscall.Setgid(gi)
	if err != nil {
		log.Fatalf("unable to guid for %v: %v", gi, err)
	}
	err = syscall.Setuid(ui)
	if err != nil {
		log.Fatalf("unable to suid for %v: %v", gi, err)
	}
	log.Printf("* Setuid to UID=%d GID=%d", os.Geteuid(), os.Getgid())
}

func chRoot() {
	if !*chroot {
		return
	}
	err := syscall.Chroot(*rootDir)
	if err != nil {
		log.Fatal("chroot: ", err)
	}
	log.Print("* Chroot to: ", *rootDir)
	*rootDir = "/"
}
