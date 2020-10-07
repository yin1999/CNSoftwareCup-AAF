package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	execGo = "/usr/bin/go"
	execPy = "/usr/local/bin/pylint"
)

type execErr struct {
	cmd     string
	errMsg  string
	errCode int
}

func builder(p programInfo) error {
	switch p.file {
	case python2:
		return py2Builder(pwd + p.dir)
	case python3:
		return py3Builder(pwd + p.dir)
	case golang:
		return goBuilder(pwd + p.dir)
	default:
		return errTypeErr
	}
}

func py2Builder(dir string) error {
	if err := resolveDepends(dir); err != nil {
		return err
	}
	return dockerRunCmd(python3, dir)
}

func py3Builder(dir string) error {
	if err := resolveDepends(dir); err != nil {
		return err
	}
	return dockerRunCmd(python3, dir)
}

func goBuilder(dir string) error {
	// create go.mod
	cmd := exec.Cmd{
		Path: execGo,
		Args: []string{execGo, "mod", "init", "main"},
		Dir:  dir,
	}
	err := runCmd(&cmd)
	if err != nil {
		return err
	}

	// build
	cmd = exec.Cmd{
		Path: execGo,
		Args: []string{execGo, "build", "-ldflags", "-s -w"},
		Dir:  dir,
	}
	err = runCmd(&cmd)
	return err
}

func (t execErr) Error() string {
	return fmt.Sprintf("cmd: %s return %d, errMsg: %s", t.cmd, t.errCode, t.errMsg)
}

func runCmd(cmd *exec.Cmd) error {
	stdOut, _ := cmd.StdoutPipe()
	var err error
	if err = cmd.Start(); err != nil {
		return err
	}
	out, _ := ioutil.ReadAll(stdOut)
	err = cmd.Wait()
	if err != nil {
		var cmdStr string
		if len(cmd.Args) == 0 {
			cmdStr = cmd.Path
		} else {
			cmdStr = cmd.Path + " " + cmd.Args[0]
		}
		return execErr{
			cmd:     cmdStr,
			errMsg:  string(out),
			errCode: cmd.ProcessState.ExitCode(),
		}
	}
	return err
}

func resolveDepends(dir string) error {
	if dir[len(dir)-1] != filepath.Separator {
		dir = string(append([]byte(dir), filepath.Separator))
	}
	out, err := os.OpenFile(dir+"requirements.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer out.Close()
	f, err := os.Open(dir + "main.py")
	if err != nil {
		return err
	}
	defer f.Close()
	var line string
	fmt.Fscanln(f, &line)
	if line != "# requests" {
		return nil
	}
	for {
		fmt.Fscanln(f, &line)
		if line == "# end" {
			return nil
		}
		out.WriteString(line[1:] + "\n")
	}
}
