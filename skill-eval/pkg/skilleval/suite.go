package skilleval

import (
	"fmt"
	"sync"
	"time"
)

// WorkerResult holds the outcome of one eval execution including artifact paths
// and any artifact-write error (distinct from eval execution errors).
// In single mode, Result is populated. In compare mode, Compare is populated.
type WorkerResult struct {
	Result   EvalResult     // populated in single mode
	Compare  *CompareResult // non-nil in compare mode
	Paths    ArtifactPaths
	WriteErr error // non-nil if artifact files could not be written
}

// Suite orchestrates concurrent execution of multiple evals.
// At Concurrency 1, behavior is equivalent to sequential execution.
// When Compare is true, each eval runs twice (with and without prompt) and is classified.
type Suite struct {
	Evaluator   *Evaluator
	Concurrency int
	Compare     bool
}

// Run executes all evals using a worker pool of Suite.Concurrency goroutines.
// onComplete fires on the result-collection goroutine (caller's goroutine) as each
// eval finishes. completed is 1-indexed. Results are returned in completion order;
// sort by eval ID if deterministic output is needed.
//
// Note: high Concurrency values launch many simultaneous claude -p subprocesses.
// Compare mode doubles the subprocess count per eval. The caller is responsible for
// understanding their API subscription's concurrency tolerance.
func (s *Suite) Run(evals []Eval, onComplete func(completed, total int, wr WorkerResult)) []WorkerResult {
	total := len(evals)
	if total == 0 {
		return nil
	}

	concurrency := s.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}

	taskCh := make(chan Eval, total)
	resultCh := make(chan WorkerResult, total)

	for _, eval := range evals {
		taskCh <- eval
	}
	close(taskCh)

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for eval := range taskCh {
				if s.Compare {
					resultCh <- s.runOneCompare(eval)
				} else {
					resultCh <- s.runOneSingle(eval)
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	results := make([]WorkerResult, 0, total)
	completed := 0
	for wr := range resultCh {
		completed++
		if onComplete != nil {
			onComplete(completed, total, wr)
		}
		results = append(results, wr)
	}

	return results
}

// runOneSingle executes a single eval end-to-end within a worker: prepares the
// artifact directory, runs the eval, writes artifacts.
func (s *Suite) runOneSingle(eval Eval) WorkerResult {
	cfg := s.Evaluator.Config
	ts := time.Now().UTC()
	paths := BuildArtifactPaths(cfg.ResultsDir, eval.Tests, eval.ID, ts)
	wr := WorkerResult{Paths: paths}

	if err := PrepareEvalDir(paths.Dir); err != nil {
		wr.Result = EvalResult{
			EvalID:  eval.ID,
			TestsID: eval.Tests,
			Model:   cfg.DefaultModel,
			RanAt:   ts,
			Err:     fmt.Errorf("preparing artifact dir: %w", err),
		}
		return wr
	}

	wr.Result = s.Evaluator.Run(eval)

	if wr.Result.Err == nil {
		if err := WriteWithPromptMD(paths.WithPromptMD, wr.Result.Output); err != nil {
			wr.WriteErr = err
		}
	}
	if err := WriteResultYML(paths, wr.Result); err != nil && wr.WriteErr == nil {
		wr.WriteErr = err
	}

	return wr
}

// runOneCompare executes both the with-prompt and without-prompt runs for an eval,
// writes both markdown artifacts and the extended compare result YAML.
func (s *Suite) runOneCompare(eval Eval) WorkerResult {
	cfg := s.Evaluator.Config
	ts := time.Now().UTC()
	paths := BuildArtifactPaths(cfg.ResultsDir, eval.Tests, eval.ID, ts)
	wr := WorkerResult{Paths: paths}

	if err := PrepareEvalDir(paths.Dir); err != nil {
		cr := CompareResult{
			EvalID:  eval.ID,
			TestsID: eval.Tests,
			Model:   cfg.DefaultModel,
			RanAt:   ts,
			Err:     fmt.Errorf("preparing artifact dir: %w", err),
		}
		wr.Compare = &cr
		return wr
	}

	cr := s.Evaluator.RunCompare(eval)
	wr.Compare = &cr

	if cr.WithPrompt.Err == nil {
		if err := WriteWithPromptMD(paths.WithPromptMD, cr.WithPrompt.Output); err != nil {
			wr.WriteErr = err
		}
	}
	if cr.WithoutPrompt.Err == nil {
		if err := WriteWithoutPromptMD(paths.WithoutPromptMD, cr.WithoutPrompt.Output); err != nil && wr.WriteErr == nil {
			wr.WriteErr = err
		}
	}
	if err := WriteCompareResultYML(paths, cr); err != nil && wr.WriteErr == nil {
		wr.WriteErr = err
	}

	return wr
}
