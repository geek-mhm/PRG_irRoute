package system

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type Runner interface {
	Run(name string, args ...string) (string, error)
	RunInput(input, name string, args ...string) (string, error)
	LookPath(name string) error
	IsLinux() bool
	IsRoot() bool
}

type HostRunner struct{}

func (HostRunner) Run(name string, args ...string) (string, error) {
	return runCommand("", name, args...)
}

func (HostRunner) RunInput(input, name string, args ...string) (string, error) {
	return runCommand(input, name, args...)
}

func (HostRunner) LookPath(name string) error {
	_, err := exec.LookPath(name)
	return err
}

func (HostRunner) IsLinux() bool {
	return runtime.GOOS == "linux"
}

func (HostRunner) IsRoot() bool {
	return os.Geteuid() == 0
}

func runCommand(input, name string, args ...string) (string, error) {
	command := exec.Command(name, args...)
	if input != "" {
		command.Stdin = strings.NewReader(input)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	output := strings.TrimSpace(stdout.String())
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return output, fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), message)
	}
	return output, nil
}
