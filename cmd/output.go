package cmd

import (
	"io"
	"os"
)

var (
	outWriterFunc = func() io.Writer { return os.Stdout }
	errWriterFunc = func() io.Writer { return os.Stderr }
)

func init() {
	outWriterFunc = func() io.Writer { return rootCmd.OutOrStdout() }
	errWriterFunc = func() io.Writer { return rootCmd.ErrOrStderr() }
}

func outWriter() io.Writer {
	return outWriterFunc()
}

func errWriter() io.Writer {
	return errWriterFunc()
}
