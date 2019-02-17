# Go Interrupter

Enables EINTR at specific thread by sending signal in Golang.

Golang runtime installs all sigaction with SA_RESTART flag. ([signal : Go programs that use cgo or SWIG](https://golang.org/pkg/os/signal/#hdr-Go_programs_that_use_cgo_or_SWIG))

Signal with SA_RESTART flag does not raise EINTR error on blocking system call so that blocking system call (such as `flock`) will never returns.

`gointr.Interrupter` override sigaction with empty (OS default) flags on `Setup()`, and reset with Golang runtime default sigaction on `Close()`.

`gointr.Interrupter` interrupt and raise EINTR error to blocking (and already `Setup()`ed) thread on `Signal()` by sending signal.

## Usage

Example : file lock with `fcntl`

```go
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kawasin73/gointr"
)

// lock locks file using FCNTL.
func lock(file *os.File) error {
	// write lock whole file
	flock := syscall.Flock_t{
		Start:  0,
		Len:    0,
		Type:   syscall.F_WRLCK,
		Whence: io.SeekStart,
	}

	if err := syscall.FcntlFlock(file.Fd(), syscall.F_SETLKW, &flock); err != nil {
		// FCNTL returns EINTR if interrupted by signal on blocking mode
		if err == syscall.EINTR {
			return fmt.Errorf("file lock timeout for %q", file.Name())
		}
		return &os.PathError{Op: "fcntl", Path: file.Name(), Err: err}
	}
	return nil
}

func main() {
	signal.Ignore()

	file, err := os.Create("./.lock")
	if err != nil {
		log.Panic(err)
	}

	// init pthread
	intr := gointr.New(syscall.SIGUSR1)

	// init error channel
	chErr := make(chan error, 1)

	// setup timer
	timer := time.NewTimer(3 * time.Second)

	go func() {
		// setup the thread signal settings
		if terr := intr.Setup(); terr != nil {
			chErr <- terr
			return
		}
		defer func() {
			// reset signal settings
			if terr := intr.Close(); terr != nil {
				// if failed to reset sigaction, go runtime will be broken.
				// terr occurs on C memory error which does not happen.
				panic(terr)
			}
		}()
		// lock file blocking
		chErr <- lock(file)
	}()

	for {
		select {
		case err = <-chErr:
			timer.Stop()
			if err == nil {
				log.Println("lock success")
			} else {
				log.Println("lock fail err", err)
			}
			// break loop
			return

		case <-timer.C:
			log.Println("timeout")

			// send signal to the thread locking file and unblock the lock with EINTR
			err := intr.Signal()
			log.Println("signal")
			if err != nil {
				log.Panic("failed to kill thread", err)
			}
			// wait for lock result from chErr
		}
	}
}

``` 

## Notes

This library uses **CGO** to specify OS thread and send signal to the thread by `pthread_self()` and `pthread_kill`

`gointr` uses global variable to store setup thread and Golang runtime default sigaction. You **SHOULD NOT** `Setup()` multiple `gointr.Interrupter` at same time. 

## why use `pthread_kill()`

signal can be sent to self process by using `syscall.Kill()`. 

But `syscall.Kill()` sends signal to self process with **process scope** not thread scope, so that other thread may handle sigaction and the blocking thread is not interrupted and raise no `EINTR`.

In order to handle sigaction at the specific thread, it requires to use `pthread_kill()` which sends signal to specific thread.  

## LICENSE

MIT