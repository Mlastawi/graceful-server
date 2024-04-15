package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	// prepare pooler
	pooler := NewPooler()

	mux := http.NewServeMux()
	// handler for requests
	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		log.Println("DEBUG: handling request")
		if status, err := pooler.handleJob(request.Context()); err != nil {
			http.Error(writer, err.Error(), status)
			return
		}

		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte("cool"))
	})

	// prepare a server listening on 8080 port
	srv := http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// run server
	go func() {
		log.Println("INFO: running...")
		pooler.Run()
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("ERROR: %v", err)
		}
	}()

	shutdownChan := make(chan os.Signal)
	signal.Notify(shutdownChan, os.Interrupt, os.Kill)

	switch sig := <-shutdownChan; sig {
	case os.Interrupt:
		log.Println("INFO: gracefully closing server")
		pooler.Shutdown(true)
	case os.Kill:
		log.Println("INFO: forcefully closing server")
		pooler.Shutdown(false)
	}

	log.Println("INFO: stopped server")

	if err := srv.Close(); err != nil {
		log.Printf("ERROR: %v", err)
	}
}

// CancellableContext is a helper interface for contexts with cancel function
type CancellableContext interface {
	context.Context
	Cancel()
}

type cancellableContext struct {
	context.Context
	cancelFn func()
}

func (c *cancellableContext) Cancel() {
	if c != nil && c.cancelFn != nil {
		c.cancelFn()
	}
}

type Pooler struct {
	running atomic.Bool // is the Pooler accepting requests

	jobs      map[int64]CancellableContext // map of currently running jobs
	jobsMutex sync.Mutex                   // mutex to sync jobs creation/completion
	nextID    atomic.Int64                 // id of the next job
}

func NewPooler() *Pooler {
	p := &Pooler{
		running: atomic.Bool{},
		nextID:  atomic.Int64{},

		jobsMutex: sync.Mutex{},
		jobs:      map[int64]CancellableContext{},
	}

	return p
}

func (p *Pooler) Run() {
	p.running.Store(true)
}

func (p *Pooler) Shutdown(graceful bool) {
	const (
		gracefulTimeout       = 2 * time.Second        // how long should the server try to gracefully shutdown
		gracefulCheckInterval = 500 * time.Millisecond // how often check for graceful shutdown completion
	)

	// set runnning flag to false to stop accepting new requests
	p.running.Store(false)

	// check is any job is still running, if not - return
	if p.runningJobs() == 0 {
		return
	}

	// create graceful shutdown context after which the shutdown is forced
	shutdownContext, cancel := context.WithTimeout(context.Background(), gracefulTimeout)
	defer cancel()

	// define ticker, check for job completion every second
	ticker := time.NewTicker(gracefulCheckInterval)
	defer ticker.Stop()

	// loop until shutdown is still graceful
	for graceful {
		select {
		// if graceful shutdown context expires don't be graceful anymore
		case <-shutdownContext.Done():
			graceful = false
			log.Printf("DEBUG: no longer try to gracefully shutdown server")
		// check for jobs completion on every tick
		case <-ticker.C:
			if p.runningJobs() == 0 {
				return
			}
		}
	}

	// if the shutdown is no longer graceful, loop over running jobs and cancel them
	p.jobsMutex.Lock()
	for id, jCtx := range p.jobs {
		jCtx.Cancel()
		delete(p.jobs, id)
	}
	p.jobsMutex.Unlock()
}

// runningJobs returns how many jobs are running in Pooler
func (p *Pooler) runningJobs() int {
	p.jobsMutex.Lock()
	defer p.jobsMutex.Unlock()
	return len(p.jobs)
}

// handleJob checks if Pooler is still running and invokes the job
func (p *Pooler) handleJob(ctx context.Context) (int, error) {
	const jobTimeout = 5 * time.Second

	// check if pooler should accept the request
	if p.running.Load() == false {
		return http.StatusServiceUnavailable, errors.New("server is not accepting requests")
	}

	// take the job ID
	id := p.nextID.Load()
	p.nextID.Add(1)

	// prepare job's context with timeout
	ctx, cancel := context.WithTimeout(ctx, jobTimeout)
	cctx := &cancellableContext{
		Context:  ctx,
		cancelFn: cancel,
	}

	// clear job after completion
	defer func() {
		cctx.Cancel()
		p.jobsMutex.Lock()
		delete(p.jobs, id)
		p.jobsMutex.Unlock()
	}()

	// store job to run
	p.jobsMutex.Lock()
	p.jobs[id] = cctx
	p.jobsMutex.Unlock()

	// run job
	if err := dummyJob(ctx); err != nil {
		return http.StatusInternalServerError, fmt.Errorf("job ID: %v failed: %w", id, err)
	}

	return http.StatusOK, nil
}

// dummyJob is a dummy job for demo purposes
/*
Function takes a random amount of time following a gaussian distribution (capped at 0 seconds at lower band).
If the context ends first it returns an error with context's explanation. Otherwise, it returns nil.
*/
func dummyJob(ctx context.Context) error {
	const (
		mean   = 2
		stddev = 1
	)

	t := (rand.NormFloat64() + mean) * stddev
	if t < 0 { // cap at 0, so time.Duration is not negative
		t = 0
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(t * float64(time.Second))):
		return nil
	}

	return nil
}
