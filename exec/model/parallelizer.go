package model

import "sync"

const (
	maxWorkers = 64 //magic number
)

type DoWorkFunc func(workID int)

func ParallelizeExec(workCount int, doWork DoWorkFunc) {
	workers := maxWorkers
	toExec := make(chan int, workCount)

	for i := 0; i < workCount; i++ {
		toExec <- i
	}
	close(toExec)

	if workCount < workers {
		workers = workCount
	}

	wg := sync.WaitGroup{}
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for workID := range toExec {
				doWork(workID)
			}
		}()
	}
	wg.Wait()
}
