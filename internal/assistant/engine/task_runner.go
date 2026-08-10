package adk

import (
	"context"
	"sync"

	adkplatform "google.golang.org/adk/v2/platform"
)

const maxGoogleADKParallelTasks = 10

func googleADKTaskRunnerContext(ctx context.Context) context.Context {
	return adkplatform.WithTaskRunner(ctx, boundedGoogleADKTaskRunner(maxGoogleADKParallelTasks))
}

func boundedGoogleADKTaskRunner(limit int) adkplatform.TaskRunner {
	if limit <= 0 {
		limit = 1
	}
	return func(ctx context.Context, tasks []func(context.Context)) {
		if len(tasks) == 0 {
			return
		}
		// ADK waits for every submitted task to report completion. Queue all
		// tasks even when ctx is already cancelled so its barrier cannot stall.
		jobs := make(chan func(context.Context), len(tasks))
		for _, task := range tasks {
			jobs <- task
		}
		close(jobs)

		var workers sync.WaitGroup
		workers.Add(min(limit, len(tasks)))
		for range min(limit, len(tasks)) {
			go func() {
				defer workers.Done()
				for task := range jobs {
					task(ctx)
				}
			}()
		}
		workers.Wait()
	}
}
