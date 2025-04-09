package main

var CurrentCommit string

// BuildVersion is the local build version
const BuildVersion = "1.32.2"

func UserVersion() string {
	return BuildVersion + "+git." + CurrentCommit
}
