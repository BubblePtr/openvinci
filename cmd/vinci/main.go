// Command vinci turns a prompt into a local image file with one shell call.
//
// main() is deliberately a one-line wrapper: run() is the only seam, so the
// whole CLI contract (exit code, stdout, stderr, files written) is testable
// without any test-only injection point.
package main

import (
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, promptStdin(os.Stdin), os.Stdout, os.Stderr))
}

// promptStdin hides an interactive terminal from run(). Reading a prompt from
// a tty would hang the CLI waiting for a human, so the main path substitutes an
// empty reader and lets run() report the missing prompt as a usage error.
func promptStdin(f *os.File) io.Reader {
	info, err := f.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice != 0 {
		return emptyReader{}
	}
	return f
}

type emptyReader struct{}

func (emptyReader) Read([]byte) (int, error) { return 0, io.EOF }
