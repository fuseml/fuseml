package helpers

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"time"

	"github.com/codeskyblue/kexec"
	"github.com/pkg/errors"

	"github.com/fuseml/fuseml/cli/paas/ui"
)

type ExternalFuncWithString func() (output string, err error)

type ExternalFunc func() (err error)

func RunProc(cmd, dir string, toStdout bool) (string, error) {
	return RunProcEnv(cmd, dir, toStdout, make([]string, 0))
}

// run process with the list of env variables
func RunProcEnv(cmd, dir string, toStdout bool, env []string) (string, error) {
	if os.Getenv("DEBUG") == "true" {
		fmt.Println("Executing ", cmd)
	}
	p := kexec.CommandString(cmd)

	var b bytes.Buffer
	if toStdout {
		p.Stdout = io.MultiWriter(os.Stdout, &b)
		p.Stderr = io.MultiWriter(os.Stderr, &b)
	} else {
		p.Stdout = &b
		p.Stderr = &b
	}
	for _, e := range env {
		p.Env = append(os.Environ(), e)
	}

	p.Dir = dir

	if err := p.Run(); err != nil {
		return b.String(), err
	}

	err := p.Wait()
	return b.String(), err
}

func RunProcNoErr(cmd, dir string, toStdout bool) (string, error) {
	if os.Getenv("DEBUG") == "true" {
		fmt.Println("Executing ", cmd)
	}
	p := kexec.CommandString(cmd)

	var b bytes.Buffer
	if toStdout {
		p.Stdout = io.MultiWriter(os.Stdout, &b)
		p.Stderr = nil
	} else {
		p.Stdout = &b
		p.Stderr = nil
	}

	p.Dir = dir

	if err := p.Run(); err != nil {
		return b.String(), err
	}

	err := p.Wait()
	return b.String(), err
}

// CreateTmpFile creates a temporary file on the disk with the given contents
// and returns the path to it and an error if something goes wrong.
func CreateTmpFile(contents string) (string, error) {
	tmpfile, err := ioutil.TempFile("", "fuseml")
	if err != nil {
		return tmpfile.Name(), err
	}
	if _, err := tmpfile.Write([]byte(contents)); err != nil {
		return tmpfile.Name(), err
	}
	if err := tmpfile.Close(); err != nil {
		return tmpfile.Name(), err
	}

	return tmpfile.Name(), nil
}

// Kubectl invoces the `kubectl` command in PATH, running the specified command.
// It returns the command output and/or error.
func Kubectl(command string) (string, error) {
	_, err := exec.LookPath("kubectl")
	if err != nil {
		return "", errors.Wrap(err, "kubectl not in path")
	}

	currentdir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	cmd := fmt.Sprintf("kubectl " + command)

	return RunProc(cmd, currentdir, false)
}

func WaitForCommandCompletion(ui *ui.UI, message string, funk ExternalFuncWithString) (string, error) {
	s := ui.Progressf(" %s", message)
	defer s.Stop()

	return funk()
}

// ExecToSuccessWithTimeout retries the given function with stirng & error return,
// until it either succeeds of the timeout is reached. It retries every "interval" duration.
func ExecToSuccessWithTimeout(funk ExternalFuncWithString, timeout, interval time.Duration) (string, error) {
	timeoutChan := time.After(timeout)
	for {
		select {
		case <-timeoutChan:
			return "", errors.New(fmt.Sprintf("Timed out after %s", timeout.String()))
		default:
			if out, err := funk(); err != nil {
				time.Sleep(interval)
			} else {
				return out, nil
			}
		}
	}
}

// RunToSuccessWithTimeout retries the given function with error return,
// until it either succeeds or the timeout is reached. It retries every "interval" duration.
func RunToSuccessWithTimeout(funk ExternalFunc, timeout, interval time.Duration) error {
	timeoutChan := time.After(timeout)
	for {
		select {
		case <-timeoutChan:
			return fmt.Errorf("Timed out after %s", timeout.String())
		default:
			if err := funk(); err != nil {
				time.Sleep(interval)
			} else {
				return nil
			}
		}
	}
}
