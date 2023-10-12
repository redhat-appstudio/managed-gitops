package downloadjob

import "fmt"

const (
	// set 'workerDebug' to true if debugging the worker logic
	workerDebug = false
)

type downloadURLWorkerEntry struct {
	url  string
	path string
}

func worker(id int, jobs <-chan downloadURLWorkerEntry, results chan<- string) {
	for jobEntry := range jobs {

		if workerDebug {
			fmt.Println("worker", id, "started  job", jobEntry)
		}

		err := downloadAsFile(jobEntry.url, jobEntry.path)

		var errStr string
		if err != nil {
			errStr = err.Error()
		}

		if workerDebug {
			fmt.Println("worker", id, "finished job", jobEntry)
		}

		results <- errStr
	}
}

func downloadURLsMultithreaded(urls []downloadURLWorkerEntry) {

	numJobs := len(urls)
	jobs := make(chan downloadURLWorkerEntry, numJobs)
	results := make(chan string, numJobs)

	for w := 1; w <= 5; w++ {
		go worker(w, jobs, results)
	}

	for j := 0; j < numJobs; j++ {
		jobs <- urls[j]
	}
	close(jobs)

	for a := 1; a <= numJobs; a++ {
		errStr := <-results
		if errStr != "" {
			fmt.Println("ERROR: ", errStr)
		}
	}
}
