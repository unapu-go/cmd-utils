package cmdu

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/shell"
)

const sourceVar = "${SOURCE}"

type CmdTimeoutError struct {
	Cmd *Cmd
}

func (d CmdTimeoutError) Cause() error {
	return context.DeadlineExceeded
}

func (d *CmdTimeoutError) Error() string {
	return fmt.Sprintf("command timeout: PID=%d", d.Cmd.Process.Pid)
}

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
	*exec.Cmd
	done     func() <-chan struct{}
	canceler func()
	ondone   []func()
	timeout  time.Duration
}

func (r *Cmd) Timeout() time.Duration {
	return r.timeout
}

func (r *Cmd) OnDone(f ...func()) {
	r.ondone = append(r.ondone, f...)
}

func (r *Cmd) OnDoneE(f func() error) {
	r.ondone = append(r.ondone, func() {
		_ = f()
	})
}

func (r *Cmd) Signal(sig os.Signal) (err error) {
	return r.Cmd.Process.Signal(sig)
}

func (r *Cmd) Run() (err error) {
	if err = r.Start(); err != nil {
		return
	}
	return r.Wait()
}

func (r *Cmd) RunContext(ctx context.Context) (err error) {
	if err = r.StartContext(ctx); err != nil {
		return
	}
	return r.Wait()
}

func (r *Cmd) Start() (err error) {
	return r.StartContext(context.Background())
}

func (r *Cmd) StartContext(ctx context.Context) (err error) {
	if r.timeout > 0 {
		if tm, ok := ctx.Deadline(); ok {
			var dur = tm.Sub(time.Now())
			if dur < r.timeout {
				r.timeout = dur
			}
		}
		ctx2, canceler := context.WithTimeout(ctx, r.timeout)
		r.done = ctx2.Done
		r.canceler = canceler
	} else if tm, ok := ctx.Deadline(); ok {
		if r.timeout = tm.Sub(time.Now()); r.timeout <= time.Millisecond {
			return context.DeadlineExceeded
		}
		ctx2, canceler := context.WithTimeout(ctx, r.timeout)
		r.done = ctx2.Done
		r.canceler = canceler
	}

	if err = r.Cmd.Start(); err != nil {
		return
	}
	return
}

func (r *Cmd) Wait() (err error) {
	defer func() {
		for _, c := range r.ondone {
			c()
		}
	}()
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
		return &CmdTimeoutError{r}
	case err = <-done:
		return
	}
}

func (b CmdBuilder) Build(env expand.Environ) (hcmd *Cmd, err error) {
	if env == nil {
		env = expand.ListEnviron(os.Environ()...)
	}

	hcmd = &Cmd{}

	defer func() {
		if err != nil {
			for _, f := range hcmd.ondone {
				f()
			}
			hcmd = nil
		}
	}()

	envw := NewEnviron(env, nil)

	if b.Env != nil {
		for key, value := range b.Env {
			envw.SetString(key, value)
		}
	}

	if b.Script != "" {
		b.Script = strings.TrimSpace(b.Script)
		var (
			fscript *os.File
		)

		if strings.HasPrefix(b.Script, "#!/") {
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
				err = fmt.Errorf("expected line break.")
				return
			}

			args, err = shell.Fields(script, func(s string) string {
				if s == "SOURCE" {
					return sourceVar
				}
				return env.Get(s).String()
			})
			if err != nil {
				return
			}
			for i, arg := range args[1:] {
				if strings.HasPrefix(arg, sourceVar) {
					arg = "*" + strings.TrimPrefix(arg, sourceVar)
					if fscript, err = ioutil.TempFile("", arg); err != nil {
						return
					}
					defer fscript.Close()
					args[i+1] = fscript.Name()
					hcmd.OnDone(func() {
						_ = os.Remove(fscript.Name())
					})
					break
				}
			}
			b.Name = args[0]
			b.Args = append(args[1:], b.Args...)
		} else {
			b.Name = "sh"
		}

		if fscript == nil {
			if fscript, err = ioutil.TempFile("", "*.sh"); err != nil {
				return
			}
			defer fscript.Close()
			hcmd.OnDone(func() {
				_ = os.Remove(fscript.Name())
			})
			b.Args = append([]string{fscript.Name()}, b.Args...)
		}

		if _, err = fscript.WriteString(b.Script); err != nil {
			return
		}
	}

	hcmd.Cmd = exec.Command(b.Name, b.Args...)

	if b.Timeout != "" {
		if hcmd.timeout, err = time.ParseDuration(b.Timeout); err != nil {
			return
		}
	}

	if hcmd.Stdin, err = StdIn.Get(b.Stdin, nil); err != nil {
		return
	}

	if hcmd.Stdout, err = StdOut.Get(b.Stdout); err != nil {
		return
	}

	if hcmd.Stderr, err = StdErr.Get(b.Stderr); err != nil {
		return
	}

	hcmd.Env = EnvStrings(env, nil)

	return
}
