package downloadjob

import "fmt"

type downloadURLWorkerEntry struct {
	url  string
	path string
}

func worker(id int, jobs <-chan downloadURLWorkerEntry, results chan<- string) {
	for jobEntry := range jobs {
		// fmt.Println("worker", id, "started  job", jobEntry)

		err := downloadAsFile(jobEntry.url, jobEntry.path)

		var errStr string
		if err != nil {
			errStr = err.Error()
		}

		// fmt.Println("worker", id, "finished job", jobEntry)
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
