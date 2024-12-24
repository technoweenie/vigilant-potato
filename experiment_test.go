package scientist

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func longRunningExperiment() *Experiment {
	e := New("long-running")

	e.Use(func() (interface{}, error) {
		time.Sleep(200 * time.Millisecond)
		return 1, nil
	})

	e.Try(func() (interface{}, error) {
		time.Sleep(800 * time.Millisecond)
		return 1, nil
	})

	return e
}

func TestExperimentMatch(t *testing.T) {
	e := New("match")
	e.Use(func() (interface{}, error) {
		return 1, nil
	})
	e.Try(func() (interface{}, error) {
		return 1, nil
	})

	published := false
	e.Publish(func(r Result) error {
		published = true

		if !r.IsMatched() || r.IsMismatched() {
			t.Errorf("not matched")
		}

		if r.IsIgnored() {
			t.Errorf("ignored")
		}

		return nil
	})

	v, err := e.Run()
	if v != 1 {
		t.Errorf("Unexpected control value: %d", v)
	}

	if err != nil {
		t.Errorf("Unexpected control error: %v", err)
	}

	if !published {
		t.Errorf("expected Publish callback to run")
	}
}

func TestExperimentMismatchNoReturn(t *testing.T) {
	e := New("match")
	e.Use(func() (interface{}, error) {
		return 1, nil
	})
	e.Try(func() (interface{}, error) {
		return 2, nil
	})

	published := false
	e.Publish(func(r Result) error {
		published = true

		if r.IsMatched() || !r.IsMismatched() {
			t.Errorf("matched???")
		}

		if r.IsIgnored() {
			t.Errorf("ignored")
		}

		return nil
	})

	v, err := e.Run()
	if v != 1 {
		t.Errorf("Unexpected control value: %d", v)
	}

	if err != nil {
		t.Errorf("Unexpected control error: %v", err)
	}

	if !published {
		t.Errorf("expected Publish callback to run")
	}
}

func TestExperimentMismatchWithReturn(t *testing.T) {
	e := New("match")
	e.Use(func() (interface{}, error) {
		return 1, nil
	})
	e.Try(func() (interface{}, error) {
		return 2, nil
	})

	e.ErrorOnMismatches = true

	published := false
	e.Publish(func(r Result) error {
		published = true

		if r.IsMatched() || !r.IsMismatched() {
			t.Errorf("matched???")
		}

		if r.IsIgnored() {
			t.Errorf("ignored")
		}

		return nil
	})

	v, err := e.Run()
	if v != nil {
		t.Errorf("Unexpected control value: %v (%T)", v, v)
	}

	if _, ok := err.(MismatchError); !ok {
		t.Errorf("Unexpected control error: %v", err)
	}

	if !published {
		t.Errorf("expected Publish callback to run")
	}
}

func TestExperimentRunBefore(t *testing.T) {
	runIf := false
	before := false

	e := New("run")
	e.Use(func() (interface{}, error) {
		return 1, nil
	})
	e.Try(func() (interface{}, error) {
		return 1, nil
	})

	e.RunIf(func() (bool, error) {
		runIf = true
		return true, nil
	})

	e.BeforeRun(func() error {
		before = true
		return nil
	})

	v, err := e.Run()
	if v != 1 {
		t.Errorf("Unexpected control value: %d", v)
	}

	if err != nil {
		t.Errorf("Unexpected control error: %v", err)
	}

	if !runIf {
		t.Errorf("expected RunIf callback to run")
	}

	if !before {
		t.Errorf("expected BeforeRun callback to run")
	}
}

func TestExperimentDisabledRunBefore(t *testing.T) {
	runIf := false

	e := New("run")
	e.Use(func() (interface{}, error) {
		return 1, nil
	})
	e.Try(func() (interface{}, error) {
		return 1, nil
	})

	e.RunIf(func() (bool, error) {
		runIf = true
		return false, nil
	})

	e.BeforeRun(func() error {
		t.Errorf("did not expect BeforeRun callback to run")
		return nil
	})

	v, err := e.Run()
	if v != 1 {
		t.Errorf("Unexpected control value: %d", v)
	}

	if err != nil {
		t.Errorf("Unexpected control error: %v", err)
	}

	if !runIf {
		t.Errorf("expected RunIf callback to run")
	}
}

func TestExperimentEmptyRunBefore(t *testing.T) {
	runIf := false

	e := New("run")
	e.Use(func() (interface{}, error) {
		return 1, nil
	})

	e.RunIf(func() (bool, error) {
		runIf = true
		return true, nil
	})

	e.BeforeRun(func() error {
		t.Errorf("did not expect BeforeRun callback to run")
		return nil
	})

	v, err := e.Run()
	if v != 1 {
		t.Errorf("Unexpected control value: %d", v)
	}

	if err != nil {
		t.Errorf("Unexpected control error: %v", err)
	}

	if !runIf {
		t.Errorf("expected RunIf callback to run")
	}
}

func TestExperimentRunIfError(t *testing.T) {
	reported := false
	e := New("run")
	e.Use(func() (interface{}, error) {
		return 1, nil
	})

	e.Try(func() (interface{}, error) {
		t.Errorf("did not expect to run experiment if RunIf() returns error")
		return 1, nil
	})

	e.Publish(func(r Result) error {
		t.Errorf("did not expect to publish")
		return nil
	})

	e.ReportErrors(func(errors ...ResultError) {
		for _, err := range errors {
			switch err.Operation {
			case "run_if":
				reported = true
				if err.Experiment != e.Name {
					t.Errorf("Bad experiment name for %q operation: %q", err.Operation, err.Experiment)
				}
				if actual := err.Error(); actual != "run_if" {
					t.Errorf("Bad error message for run_if operation: %q", actual)
				}
			default:
				t.Errorf("Bad operation: %q", err.Operation)
			}
		}
	})

	e.RunIf(func() (bool, error) {
		return true, fmt.Errorf("run_if")
	})

	v, err := e.Run()
	if v != nil {
		t.Errorf("unexpected result: %v", v)
	}

	if err == nil {
		t.Errorf("expected a run_if error!")
	} else if err.Error() != "run_if" {
		t.Errorf("unexpected error: %v", err.Error())
	}

	if !reported {
		t.Errorf("result errors never reported!")
	}
}

func TestExperimentSkipCompareMismatchedValues(t *testing.T) {
	e := New("ignore")
	e.Use(func() (interface{}, error) {
		return 1, nil
	})
	e.Try(func() (interface{}, error) {
		return 2, nil
	})
	e.Compare(func(control, candidate interface{}) (bool, error) {
		return true, nil
	})

	published := false
	e.Publish(func(r Result) error {
		published = true

		if r.IsMismatched() {
			t.Errorf("Should not be matching")
		}

		return nil
	})

	v, err := e.Run()
	if v != 1 {
		t.Errorf("Unexpected control value: %d", v)
	}

	if err != nil {
		t.Errorf("Unexpected control error: %v", err)
	}

	if !published {
		t.Errorf("results never published")
	}
}

func TestExperimentSkipCompareMismatchedErrors(t *testing.T) {
	e := New("ignore")
	e.Use(func() (interface{}, error) {
		return 1, nil
	})
	e.Try(func() (interface{}, error) {
		return 1, errors.New("try")
	})
	e.Compare(func(control, candidate interface{}) (bool, error) {
		return true, nil
	})

	published := false
	e.Publish(func(r Result) error {
		published = true

		if r.IsMatched() {
			t.Errorf("Should be mismatched")
		}

		return nil
	})

	v, err := e.Run()
	if v != 1 {
		t.Errorf("Unexpected control value: %d", v)
	}

	if err != nil {
		t.Errorf("Unexpected control error: %v", err)
	}

	if !published {
		t.Errorf("results never published")
	}
}

func TestExperimentSkipCompareSameErrors(t *testing.T) {
	e := New("ignore")
	e.Use(func() (interface{}, error) {
		return 1, errors.New("ok")
	})
	e.Try(func() (interface{}, error) {
		return 1, errors.New("ok")
	})
	e.Compare(func(control, candidate interface{}) (bool, error) {
		return true, nil
	})

	published := false
	e.Publish(func(r Result) error {
		published = true

		if r.IsMismatched() {
			t.Errorf("Should be matching")
		}

		return nil
	})

	v, err := e.Run()
	if v != 1 {
		t.Errorf("Unexpected control value: %d", v)
	}

	if err == nil || err.Error() != "ok" {
		t.Errorf("Unexpected control error: %v", err)
	}

	if !published {
		t.Errorf("results never published")
	}
}

func TestSequentialExecution(t *testing.T) {
	e := longRunningExperiment()
	startTime := time.Now()

	_, err := e.Run()
	duration := time.Since(startTime)

	if err != nil {
		t.Errorf("Unexpected control error: %v", err)
	}

	if duration < time.Second {
		t.Errorf("Expected experiment to take at least 1 second, took %s", duration)
	}
}

func TestConcurrentExecution(t *testing.T) {
	e := longRunningExperiment()
	startTime := time.Now()
	e.EnableConcurrency(nil)

	_, err := e.Run()
	duration := time.Since(startTime)

	if err != nil {
		t.Errorf("Unexpected control error: %v", err)
	}

	if duration < 800*time.Millisecond || duration > 810*time.Millisecond {
		t.Errorf("Expected experiment to take 800ms, took %s", duration)
	}
}

func TestConcurrentExecutionWithTimeout(t *testing.T) {
	e := longRunningExperiment()
	timeout := time.Duration(500) * time.Millisecond
	e.EnableConcurrency(&timeout)

	_, err := e.Run()

	e.Publish(func(r Result) error {
		if r.Errors == nil || len(r.Errors) == 0 {
			t.Errorf("Expected timeout error")
		}
		if r.Errors[0].Operation != "timeout" {
			t.Errorf("Expected timeout error")
		}
		return nil
	})

	if err != nil {
		t.Errorf("Unexpected control error: %v", err)
	}
}
