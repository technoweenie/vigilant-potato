package scientist

import (
	"fmt"
	"os"
	"reflect"
	"time"
)

var ErrorOnMismatches bool

func New(name string) *Experiment {
	return &Experiment{
		Name:              name,
		Context:           make(map[string]string),
		ErrorOnMismatches: ErrorOnMismatches,
		runConcurrently:   false,
		timeout:           nil,
		behaviors:         make(map[string]behaviorFunc),
		comparator:        defaultComparator,
		runcheck:          defaultRunCheck,
		publisher:         defaultPublisher,
		errorReporter:     defaultErrorReporter,
		beforeRun:         defaultBeforeRun,
		cleaner:           defaultCleaner,
	}
}

type behaviorFunc func() (value interface{}, err error)

type Experiment struct {
	Name              string
	Context           map[string]string
	ErrorOnMismatches bool
	runConcurrently   bool
	timeout           *time.Duration
	behaviors         map[string]behaviorFunc
	ignores           []func(control, candidate interface{}) (bool, error)
	comparator        func(control, candidate interface{}) (bool, error)
	runcheck          func() (bool, error)
	publisher         func(Result) error
	errorReporter     func(...ResultError)
	beforeRun         func() error
	cleaner           func(interface{}) (interface{}, error)
}

func (e *Experiment) Use(fn func() (interface{}, error)) {
	e.Behavior(controlBehavior, fn)
}

func (e *Experiment) Try(fn func() (interface{}, error)) {
	e.Behavior(candidateBehavior, fn)
}

func (e *Experiment) Behavior(name string, fn func() (interface{}, error)) {
	e.behaviors[name] = fn
}

func (e *Experiment) Compare(fn func(control, candidate interface{}) (bool, error)) {
	e.comparator = fn
}

func (e *Experiment) Clean(fn func(v interface{}) (interface{}, error)) {
	e.cleaner = fn
}

func (e *Experiment) Ignore(fn func(control, candidate interface{}) (bool, error)) {
	e.ignores = append(e.ignores, fn)
}

func (e *Experiment) RunIf(fn func() (bool, error)) {
	e.runcheck = fn
}

func (e *Experiment) BeforeRun(fn func() error) {
	e.beforeRun = fn
}

func (e *Experiment) Publish(fn func(Result) error) {
	e.publisher = fn
}

func (e *Experiment) ReportErrors(fn func(...ResultError)) {
	e.errorReporter = fn
}

func (e *Experiment) EnableConcurrency(timeout *time.Duration) {
	e.runConcurrently = true
	e.timeout = timeout
}

func (e *Experiment) Run() (interface{}, error) {
	return e.RunBehavior(controlBehavior)
}

func (e *Experiment) RunBehavior(name string) (interface{}, error) {
	enabled, err := e.runcheck()
	if err != nil {
		enabled = true
		e.errorReporter(e.resultErr("run_if", err))
		return nil, err
	}

	if enabled && len(e.behaviors) > 1 {
		r := Run(e, name)

		if r.Control.Err == nil && e.ErrorOnMismatches && r.IsMismatched() {
			return nil, MismatchError{r}
		}

		return r.Control.Value, r.Control.Err
	}

	behavior, ok := e.behaviors[name]
	if !ok {
		return nil, behaviorNotFound(e, name)
	}

	return behavior()
}

func (e *Experiment) resultErr(name string, err error) ResultError {
	return ResultError{name, e.Name, err}
}

func defaultComparator(candidate, control interface{}) (bool, error) {
	return reflect.DeepEqual(candidate, control), nil
}

func defaultRunCheck() (bool, error) {
	return true, nil
}

func defaultCleaner(v interface{}) (interface{}, error) {
	return v, nil
}

func defaultPublisher(r Result) error {
	return nil
}

func defaultErrorReporter(errs ...ResultError) {
	for _, err := range errs {
		fmt.Fprintf(os.Stderr, "[scientist] error during %q for %q experiment: (%T) %v\n", err.Operation, err.Experiment, err.Err, err.Err)
	}
}

func defaultBeforeRun() error {
	return nil
}
