package cmdu

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"mvdan.cc/sh/v3/expand"
)

type CmdBuilder struct {
	Script  string            `yaml:"script"`
	Name    string            `yaml:"name"`
	Env     map[string]string `yaml:"env"`
	Args    []string          `yaml:"args"`
	Dir     string            `yaml:"dir"`
	Stdout  string            `yaml:"stdout"`
	Stderr  string            `yaml:"stderr"`
	Stdin   string            `yaml:"stdin"`
	Timeout string            `yaml:"timeout"`
}

type Cmd struct {
	Cmd      *exec.Cmd
	done     func() <-chan struct{}
	canceler func()
}

func (r *Cmd) Kill(sig os.Signal) (err error) {
	return r.Cmd.Process.Signal(sig)
}

func (r *Cmd) Start() (err error) {
	return r.Cmd.Start()
}

func (r *Cmd) Wait() (err error) {
	if r.canceler == nil {
		return r.Cmd.Wait()
	}

	defer r.canceler()
	var done = make(chan error, 1)
	go func() {
		done <- r.Cmd.Wait()
	}()
	select {
	case <-r.done():
		r.Cmd.Process.Signal(syscall.SIGTERM)
	case err = <-done:
		return
	}
	return
}

func (b CmdBuilder) Build(ctx context.Context, env expand.Environ) (hcmd *Cmd, err error) {
	if env == nil {
		env = expand.ListEnviron(os.Environ()...)
	}

	envw := NewEnviron(env, nil)

	if b.Env != nil {
		for key, value := range b.Env {
			envw.SetString(key, value)
		}
	}

	if b.Script != "" {
		if strings.HasPrefix(b.Script, "#!") {
			b.Script = b.Script[2:]
			var (
				args   []string
				script string
				elpos  = strings.IndexByte(b.Script, '\n')
			)

			if elpos > 0 {
				script = b.Script[0:elpos]
				b.Script = b.Script[elpos+1:]
			} else {
				return nil, fmt.Errorf("expected line break.")
			}

			script = strings.TrimSpace(script)

			args, err = Fields(script, env)
			if err != nil {
				return nil, err
			}
			b.Name = args[0]
			b.Args = append(args[1:], b.Args...)
		} else {
			b.Name = "sh"
			b.Args = append([]string{"-s"}, b.Args...)
		}
	}

	var (
		cmd     = exec.Command(b.Name, b.Args...)
		timeout time.Duration
	)

	if b.Timeout != "" {
		if timeout, err = time.ParseDuration(b.Timeout); err != nil {
			return
		}
	}

	if b.Script != "" {
		var script bytes.Buffer
		switch b.Stdin {
		case "":
			script.WriteString(b.Script)
		default:
			var fdPth string
			if b.Stdin == "-" {
				fdPth = fmt.Sprintf("/proc/%d/fd/0", os.Getpid())
			} else {
				fdPth = b.Stdin
			}
			switch filepath.Base(b.Name) {
			case "bash", "sh":
				fmt.Fprintf(&script, "(\n%s\n) < %s\n", b.Script, fdPth)
			default:
				script.WriteString(b.Script)
				envw.SetString("STDIN", fdPth)
			}
		}
		cmd.Stdin = &script
	} else if cmd.Stdin, err = StdIn.Get(b.Stdin, nil); err != nil {
		return nil, err
	}

	if cmd.Stdout, err = StdOut.Get(b.Stdout); err != nil {
		return nil, err
	}

	if cmd.Stderr, err = StdErr.Get(b.Stderr); err != nil {
		return nil, err
	}

	cmd.Env = EnvStrings(env, nil)

	hcmd = &Cmd{Cmd: cmd}

	if timeout > 0 {
		if tm, ok := ctx.Deadline(); ok {
			var dur = time.Now().Sub(tm)
			if dur < timeout {
				timeout = dur
			}
		}
		ctx2, canceler := context.WithTimeout(ctx, timeout)
		hcmd.done = ctx2.Done
		hcmd.canceler = canceler
	} else if tm, ok := ctx.Deadline(); ok {
		timeout = time.Now().Sub(tm)
		ctx2, canceler := context.WithTimeout(ctx, timeout)
		hcmd.done = ctx2.Done
		hcmd.canceler = canceler
	}

	return hcmd, nil
}
