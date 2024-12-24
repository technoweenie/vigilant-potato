package scientist

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	controlBehavior   = "control"
	candidateBehavior = "candidate"
)

type Observation struct {
	Experiment *Experiment
	Name       string
	Started    time.Time
	Runtime    time.Duration
	Value      interface{}
	Err        error
}

func (o *Observation) CleanedValue() (interface{}, error) {
	return o.Experiment.cleaner(o.Value)
}

type Result struct {
	Experiment   *Experiment
	Control      *Observation
	Observations []*Observation
	Candidates   []*Observation
	Ignored      []*Observation
	Mismatched   []*Observation
	Errors       []ResultError
}

type observationResult struct {
	name string
	obs  *Observation
	err  error
}

func (r Result) IsMatched() bool {
	if r.IsMismatched() || r.IsIgnored() {
		return false
	}
	return true
}

func (r Result) IsMismatched() bool {
	return len(r.Mismatched) > 0
}

func (r Result) IsIgnored() bool {
	return len(r.Ignored) > 0
}

func Run(e *Experiment, name string) Result {
	r := Result{Experiment: e}
	if err := e.beforeRun(); err != nil {
		r.Errors = append(r.Errors, e.resultErr("before_run", err))
	}

	numBehaviors := len(e.behaviors)

	r.Candidates = make([]*Observation, numBehaviors-1)
	r.Ignored = make([]*Observation, 0, numBehaviors-1)
	r.Mismatched = make([]*Observation, 0, numBehaviors-1)
	r.Observations = make([]*Observation, numBehaviors)

	if !e.runConcurrently {
		r.Control = observe(e, name, e.behaviors[name])
		r.Observations[0] = r.Control

		i := 0
		for bname, b := range e.behaviors {
			if bname == name {
				continue
			}
			c := observe(e, bname, b)
			r.Candidates[i] = c
			i += 1
			r.Observations[i] = c
			processObservation(e, &r, r.Control, c)
		}
	} else {
		resultChan := make(chan observationResult, numBehaviors)
		var wg sync.WaitGroup

		for bname, b := range e.behaviors {
			wg.Add(1)
			go func(behaviorName string, behavior behaviorFunc) {
				defer wg.Done()

				var ctx context.Context
				var cancel context.CancelFunc

				ctx, cancel = createContext(e)
				defer cancel()

				doneChan := make(chan *Observation, 1)
				go func() {
					obs := observe(e, behaviorName, behavior)
					doneChan <- obs
				}()

				select {
				case obs := <-doneChan:
					resultChan <- observationResult{behaviorName, obs, nil}
				case <-ctx.Done():
					timeoutErr := ctx.Err()
					resultChan <- observationResult{
						name: behaviorName,
						obs: &Observation{
							Name: behaviorName,
						},
						err: timeoutErr,
					}
				}
			}(bname, b)
		}

		go func() {
			wg.Wait()
			close(resultChan)
		}()

		i := 0
		for res := range resultChan {
			if res.err != nil {
				r.Errors = append(r.Errors, e.resultErr("timeout", res.err))
			}

			if res.name == name {
				r.Control = res.obs
				r.Observations[0] = res.obs
				continue
			}

			r.Candidates[i] = res.obs
			r.Observations[i+1] = res.obs
			i++
		}

		for _, candidate := range r.Candidates {
			if candidate != nil {
				processObservation(e, &r, r.Control, candidate)
			}
		}
	}

	if err := e.publisher(r); err != nil {
		r.Errors = append(r.Errors, e.resultErr("publish", err))
	}

	if len(r.Errors) > 0 {
		e.errorReporter(r.Errors...)
	}

	return r
}

func createContext(e *Experiment) (context.Context, context.CancelFunc) {
	var ctx context.Context
	var cancel context.CancelFunc

	if e.timeout != nil {
		ctx, cancel = context.WithTimeout(context.Background(), *e.timeout)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	return ctx, cancel
}

func processObservation(e *Experiment, r *Result, control *Observation, candidate *Observation) {
	ok, err := matching(e, control, candidate)
	if err != nil {
		ok = false
		r.Errors = append(r.Errors, e.resultErr("compare", err))
	}

	if ok {
		return
	}

	ignored, err := ignoring(e, control, candidate)
	if err != nil {
		ignored = false
		r.Errors = append(r.Errors, e.resultErr("ignore", err))
	}

	if ignored {
		r.Ignored = append(r.Ignored, candidate)
	} else {
		r.Mismatched = append(r.Mismatched, candidate)
	}
}

func matching(e *Experiment, control, candidate *Observation) (bool, error) {
	// neither returned errors
	if control.Err == nil && candidate.Err == nil {
		return e.comparator(control.Value, candidate.Value)
	}

	// both returned errors
	if control.Err != nil && candidate.Err != nil {
		return control.Err.Error() == candidate.Err.Error(), nil
	}

	// returned different errors
	return false, nil
}

func ignoring(e *Experiment, control, candidate *Observation) (bool, error) {
	for _, i := range e.ignores {
		ok, err := i(control.Value, candidate.Value)
		if err != nil {
			return false, err
		}

		if ok {
			return true, nil
		}
	}

	return false, nil
}

func behaviorNotFound(e *Experiment, name string) error {
	return fmt.Errorf("Behavior %q not found for experiment %q", name, e.Name)
}

func observe(e *Experiment, name string, b behaviorFunc) *Observation {
	o := &Observation{
		Experiment: e,
		Name:       name,
		Started:    time.Now(),
	}

	if b == nil {
		b = e.behaviors[name]
	}

	if b == nil {
		o.Runtime = time.Since(o.Started)
		o.Err = behaviorNotFound(e, name)
	} else {
		v, err := b()
		o.Runtime = time.Since(o.Started)
		o.Value = v
		o.Err = err
	}

	return o
}

type ResultError struct {
	Operation  string
	Experiment string
	Err        error
}

func (e ResultError) Error() string {
	return e.Err.Error()
}

type MismatchError struct {
	Result Result
}

func (e MismatchError) Error() string {
	return fmt.Sprintf("[scientist] experiment %q observations mismatched", e.Result.Experiment.Name)
}
