// Build the local binary with useful metadata even in a checkout without tags.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

func git(args ...string) string {
	output, err := exec.Command("git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func main() {
	defaultOutput := "filebrowser"
	if runtime.GOOS == "windows" {
		defaultOutput += ".exe"
	}
	output := flag.String("o", defaultOutput, "output binary")
	explicitVersion := flag.String("version", "", "explicit release version (requires -commit)")
	explicitCommit := flag.String("commit", "", "explicit source commit (requires -version)")
	flag.Parse()
	commit := git("rev-parse", "--short=12", "HEAD")
	if commit == "" {
		commit = "unknown"
	}
	version := "0.0.0-dev+" + commit
	tag := git("describe", "--tags", "--exact-match", "--match", "v[0-9]*", "HEAD")
	if regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`).MatchString(tag) {
		version = strings.TrimPrefix(tag, "v")
	}
	if git("status", "--porcelain") != "" {
		if strings.Contains(version, "+") {
			version += ".dirty"
		} else {
			version += "+dirty"
		}
	}
	if (*explicitVersion == "") != (*explicitCommit == "") {
		fmt.Fprintln(os.Stderr, "-version and -commit must be provided together")
		os.Exit(2)
	}
	if *explicitVersion != "" {
		if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`).MatchString(*explicitVersion) ||
			!regexp.MustCompile(`^(?:[a-f0-9]{7,40}|unknown)$`).MatchString(*explicitCommit) {
			fmt.Fprintln(os.Stderr, "invalid explicit version or commit")
			os.Exit(2)
		}
		version, commit = *explicitVersion, *explicitCommit
	}
	fmt.Printf("Building File Browser %s (%s)\n", version, commit)
	command := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w -X github.com/filebrowser/filebrowser/v2/version.Version="+version+" -X github.com/filebrowser/filebrowser/v2/version.CommitSHA="+commit, "-o", *output, ".")
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
