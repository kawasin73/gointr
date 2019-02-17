package gointr

/*
#include <stdio.h>
#include <stddef.h>
#include <errno.h>
#include <string.h>
#include <signal.h>
#include <pthread.h>
void sighandler(int sig){
	// empty handler
}
static pthread_t tid;
static struct sigaction oact;
static int setup_signal(int sig) {
	struct sigaction act;
	// set self thread id
	tid = pthread_self();
	// setup sigaction
	act.sa_handler = sighandler;
	act.sa_flags = 0;
	sigemptyset(&act.sa_mask);
	// set sigaction and cache old sigaction to oact
	if(sigaction(sig, &act, &oact) != 0){
		return errno;
	}
	return 0;
}
static int kill_thread(int sig) {
	// send signal to thread
	if (pthread_kill(tid, sig) == -1) {
		return errno;
	}
	return 0;
}
static int reset_signal(int sig) {
	// reset with old sigaction
	if(sigaction(sig, &oact, NULL) != 0){
		return errno;
	}
	return 0;
}
*/
import "C"
import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
)

// return EFAULT when memory in C is invalid
// return EINVAL when signal is invalid
func setupSignal(sig syscall.Signal) error {
	ret := C.setup_signal(C.int(sig))
	if ret != 0 {
		return syscall.Errno(ret)
	}
	return nil
}

// return ESRCH when thread id is invalid
// return EINVAL when signal number is invalid
func killThread(sig syscall.Signal) error {
	ret := C.kill_thread(C.int(sig))
	if ret != 0 {
		return syscall.Errno(ret)
	}
	return nil
}

// return EFAULT when memory in C is invalid
// return EINVAL when signal is invalid
func resetSignal(sig syscall.Signal) error {
	ret := C.reset_signal(C.int(sig))
	if ret != 0 {
		return syscall.Errno(ret)
	}
	return nil
}


type Interrupter struct {
	mu     sync.Mutex
	cond   *sync.Cond
	sig    syscall.Signal
	init   bool
	closed bool
}

// New creates Interrupter
func New(sig syscall.Signal) *Interrupter {
	p := &Interrupter{
		sig: sig,
	}
	p.cond = sync.NewCond(&p.mu)
	return p
}

// Setup locks calling goroutine to this OS thread and overwrite sigaction with empty sa_flags (not including SA_RESTART)
// DO NOT call signal package functions until call (*Interrupter).close() or destroy sigaction with Golang runtime settings
func (intr *Interrupter) Setup() error {
	intr.mu.Lock()
	defer func() {
		intr.cond.Broadcast()
		intr.mu.Unlock()
	}()

	// set initialized flag
	intr.init = true

	// lock this goroutine to this os thread
	runtime.LockOSThread()

	// set thread id to global variable
	// overwrite sigaction of intr.sig with empty handler to remove SA_RESTART
	err := setupSignal(intr.sig)
	if err != nil {
		intr.closed = true
		return fmt.Errorf("setup sigaction : %v", err)
	}
	return nil
}

// Close
func (intr *Interrupter) Close() error {
	intr.mu.Lock()
	defer intr.mu.Unlock()
	if intr.closed {
		return nil
	}

	// unlock goroutine from os thread
	runtime.UnlockOSThread()

	// reset sigaction of intr.sig with original value which is set by Golang runtime.
	if err := resetSignal(intr.sig); err != nil {
		return fmt.Errorf("reset sigaction : %v", err)
	}
	intr.closed = true
	intr.cond.Broadcast()
	return nil
}

// Signal sends signal to the setuped thread.
func (intr *Interrupter) Signal() error {
	intr.mu.Lock()
	defer intr.mu.Unlock()
	if !intr.init {
		// wait until setup finishes.
		intr.cond.Wait()
	}
	if intr.closed {
		return nil
	}

	// send intr.sig signal to the thread.
	if err := killThread(intr.sig); err != nil {
		return fmt.Errorf("send signal to thread : %v", err)
	}
	return nil
}
