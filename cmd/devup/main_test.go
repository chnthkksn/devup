package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"devup/internal/deps"
	"devup/internal/parser"
	sshutil "devup/internal/ssh"
)

func TestCheckDependencies(t *testing.T) {
	err := deps.Check([]string{"ssh", "mutagen"}, func(file string) (string, error) {
		if file == "mutagen" {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + file, nil
	})
	if err == nil || !strings.Contains(err.Error(), `missing dependency "mutagen" in PATH`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSplitArgs(t *testing.T) {
	gotParsed, gotTarget, err := parser.SplitArgs([]string{"ubuntu@host:/apps/api", "-p", "3000", "--cmd", "npm run dev"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(gotParsed, []string{"-p", "3000", "--cmd", "npm run dev"}) || gotTarget != "ubuntu@host:/apps/api" {
		t.Fatalf("unexpected parse result: %#v %q", gotParsed, gotTarget)
	}
}

func TestParseTarget(t *testing.T) {
	got, err := parser.ParseTarget("ubuntu@example.com:/apps/api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Host != "ubuntu@example.com" || got.RemotePath != "/apps/api" {
		t.Fatalf("target mismatch: %+v", got)
	}
}

func TestParsePortMapping(t *testing.T) {
	got, err := parser.ParsePortMapping("3000:3001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != (parser.PortMapping{Local: 3000, Remote: 3001}) {
		t.Fatalf("mapping mismatch: %+v", got)
	}
}

func TestBuildSSHArgs(t *testing.T) {
	target := parser.Target{Host: "ubuntu@example.com", RemotePath: "/apps/api"}
	ports := []parser.PortMapping{{Local: 3000, Remote: 3000}, {Local: 5173, Remote: 5174}}

	got := sshutil.BuildArgs(ports, target, "", "", nil)
	want := []string{
		"-L", "3000:localhost:3000",
		"-L", "5173:localhost:5174",
		"-t", "ubuntu@example.com",
		"cd '/apps/api' && exec ${SHELL:-/bin/bash} -l",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ssh args mismatch: got %#v, want %#v", got, want)
	}
}

