package cmdu

import (
	"os"
	"path/filepath"

	ph "github.com/moisespsena-go/path-helpers"
)

type StdIOType int

const (
	StdIn StdIOType = iota
	StdOut
	StdErr
)

func (typ StdIOType) Get(pth string, defaul ...*os.File) (f *os.File, err error) {
	var sysDef bool
	if len(defaul) == 0 {
		sysDef = true
		defaul = append(defaul, nil)
	}
	switch pth {
	case "", "-":
		if defaul[0] == nil {
			if sysDef {
				switch typ {
				case StdIn:
					return os.Stdin, nil
				case StdOut:
					return os.Stdout, nil
				case StdErr:
					return  os.Stderr, nil
				}
				return
			} else {
				pth = os.DevNull
			}
		} else {
			return defaul[0], nil
		}
	case "DEV_NULL":
		pth = os.DevNull
	}
	var (
		s    os.FileInfo
		flag int
		mode os.FileMode
	)
	switch typ {
	case StdIn:
		flag = os.O_RDONLY
	case StdOut, StdErr:
		flag = os.O_APPEND | os.O_CREATE | os.O_WRONLY
	}
	if s, err = os.Stat(pth); err == nil {
		mode = s.Mode()
	} else {
		if os.IsNotExist(err) {
			dir := filepath.Dir(pth)
			if dir != "." && dir != "" {
				if err = ph.MkdirAllIfNotExists(dir); err != nil {
					return
				}
			}
			if mode, err = ph.ResolveFileMode(pth); err != nil {
				return
			}
			switch typ {
			case StdIn:
				flag = os.O_RDONLY
			case StdOut, StdErr:
				flag = os.O_APPEND | os.O_CREATE | os.O_WRONLY
			}
		} else {
			return
		}
	}
	return os.OpenFile(pth, flag, mode)
}
