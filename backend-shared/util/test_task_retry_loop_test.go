package util

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	wg sync.WaitGroup
	m  sync.Mutex
)

var _ = Describe("Task Retry Loop Unit Tests", func() {

	const (
		numberOfTasks int = 1000
	)

	var (
		workComplete chan taskRetryLoopMessage
		log          = logger.FromContext(context.Background())
	)

	BeforeEach(func() {
		workComplete = make(chan taskRetryLoopMessage)
		wg = sync.WaitGroup{}
	})

	Context("AddTaskIfNotPresent Test", func() {

		It("should rerun a test that is requesting retry", func() {

			mockTestEvent := &mockTestTaskCounter{}
			taskRetryLoop := NewTaskRetryLoop("test-name")

			wg.Add(2)
			taskRetryLoop.AddTaskIfNotPresent("my-test-task", mockTestEvent, ExponentialBackoff{Factor: 2, Min: time.Duration(100 * time.Microsecond), Max: time.Duration(1 * time.Second), Jitter: true})
			wg.Wait()

			Expect(mockTestEvent.timesRun).Should(Equal(2))
		})

		It("should generate 1000 tasks with random IDs and execute them successfully", func() {

			testEvent := &mockTestTaskEvent{shouldTaskFail: false}
			taskRetryLoop := NewTaskRetryLoop("test-name")

			for i := 0; i < numberOfTasks; i++ {
				wg.Add(1)
				taskId := uuid.New()
				taskRetryLoop.AddTaskIfNotPresent(taskId.String(), testEvent, ExponentialBackoff{Factor: 2, Min: time.Duration(100 * time.Microsecond), Max: time.Duration(1 * time.Second), Jitter: true})
			}

			fmt.Printf("Waiting for %d tasks to complete...\n", numberOfTasks)

			wg.Wait()

			fmt.Printf("%d tasks successfully completed\n", numberOfTasks)
		})
	})

	Context("AddTaskIfNotPresent Test: Test De-duplication", func() {

		It("should generate 1000 tasks with randomly selected among a list of 5 names, and the number of active tasks doesn't exceed the size of the list", func() {

			taskRetryLoop := NewTaskRetryLoop("dummy-name")
			taskNames := [5]string{"a", "b", "c", "d", "e"}

			tasksRunByName := map[string]int{}

			for i := 0; i < numberOfTasks; i++ {
				taskName := taskNames[rand.Intn(len(taskNames))]
				testEvent := &mockTestTaskEvent{shouldTaskFail: false, taskName: taskName, tasksRunByName: tasksRunByName}
				wg.Add(1)

				taskRetryLoop.AddTaskIfNotPresent(taskName, testEvent, ExponentialBackoff{Factor: 2, Min: time.Duration(100 * time.Microsecond), Max: time.Duration(1 * time.Second), Jitter: true})
			}

			fmt.Println("Waiting for tasks to complete...")

			time.Sleep(3 * time.Second)

			m.Lock()
			defer m.Unlock()

			fmt.Println(tasksRunByName)

			// Each task should have run at least once within 5 seconds
			for _, taskName := range taskNames {
				Expect(tasksRunByName[taskName]).To(BeNumerically(">", 0))
			}

			fmt.Println("Tasks successfully completed")
		})
	})

	Context("startNewTask Test", func() {

		It("ensures that calling startTask removes the task from 'waitingTasksByName'", func() {

			waitingTaskContainer := waitingTaskContainer{
				waitingTasksByName: make(map[string]any),
				waitingTasks:       []waitingTaskEntry{},
			}

			task := &mockEmptyTask{}

			activeTaskMap := make(map[string]internalTaskEntry)
			taskToStart := waitingTaskEntry{name: "test-task", task: task}

			waitingTaskContainer.waitingTasksByName["test-task"] = waitingTaskEntry{}

			Expect(waitingTaskContainer.waitingTasksByName).To(HaveLen(1))

			startNewTask(taskToStart, &waitingTaskContainer, activeTaskMap, workComplete, log)

			Expect(waitingTaskContainer.waitingTasksByName).To(BeEmpty())
		})
	})

	Context("internalTaskRunner tests", func() {

		It("ensures that the task provided runs as expected", func() {

			workComplete := make(chan taskRetryLoopMessage)
			task := &mockTestTaskEvent{shouldTaskFail: false}
			taskEntry := &internalTaskEntry{task: task, name: "test-task", creationTime: time.Now()}

			wg.Add(1)
			internalStartTaskRunner(taskEntry, workComplete, log)
			wg.Wait()

			receivedMsg := <-workComplete

			workCompletedMsg, _ := (receivedMsg.payload).(taskRetryMessage_workCompleted)

			Expect(receivedMsg.msgType).To(Equal(taskRetryLoop_workCompleted))
			Expect(workCompletedMsg.shouldRetry).Should(BeFalse())
			Expect(workCompletedMsg.resultErr).ToNot(HaveOccurred())
		})

		It("ensures that when the task returns _an error_, it is communicated to the channel in a 'taskRetryLoopMessage'", func() {

			task := &mockTestTaskEvent{shouldTaskFail: true, errorReturned: "internalTaskRunner error", shouldReturnError: true}
			taskEntry := &internalTaskEntry{task: task, name: "test-task", creationTime: time.Now()}

			wg.Add(1)
			internalStartTaskRunner(taskEntry, workComplete, log)
			wg.Wait()

			receivedMsg := <-workComplete

			workCompletedMsg, _ := (receivedMsg.payload).(taskRetryMessage_workCompleted)

			Expect(workCompletedMsg.resultErr).To(MatchError(fmt.Errorf("internalTaskRunner error")))
		})

		It("ensure that when the task return _true_ for retry, it is communicated to the channel in a 'taskRetryLoopMessage'", func() {

			task := &mockTestTaskEvent{shouldTaskFail: true, shouldReturnError: false}
			taskEntry := &internalTaskEntry{task: task, name: "test-task", creationTime: time.Now()}

			wg.Add(1)
			internalStartTaskRunner(taskEntry, workComplete, log)
			wg.Wait()

			receivedMsg := <-workComplete

			workCompletedMsg, _ := (receivedMsg.payload).(taskRetryMessage_workCompleted)

			Expect(workCompletedMsg.shouldRetry).To(BeTrue())
		})

		It("ensure that when the task returns _false_ for retry, it is communicated to the channel in a 'taskRetryLoopMessage'", func() {

			task := &mockTestTaskEvent{shouldTaskFail: false, shouldReturnError: false}
			taskEntry := &internalTaskEntry{task: task, name: "test-task", creationTime: time.Now()}

			wg.Add(1)
			internalStartTaskRunner(taskEntry, workComplete, log)
			wg.Wait()

			receivedMsg := <-workComplete

			workCompletedMsg, _ := (receivedMsg.payload).(taskRetryMessage_workCompleted)

			Expect(workCompletedMsg.shouldRetry).To(BeFalse())
		})
	})

})

// mockTestTaskCounter counts the number of calls to performTask, so that we can verify it is called a certain amount of itmes.
type mockTestTaskCounter struct {
	timesRun int
}

func (event *mockTestTaskCounter) PerformTask(taskContext context.Context) (bool, error) {
	event.timesRun++
	fmt.Println("Call: ", event.timesRun)
	defer wg.Done()

	// This method should be be called once.

	// On first call, return true for retry
	if event.timesRun == 1 {
		return true, nil
	} else if event.timesRun == 2 {
		// On second call, return false for retry
		return false, nil
	} else {
		return false, fmt.Errorf("Unexpected case")
	}

}

type mockTestTaskEvent struct {

	// Acquire mutex 'm' before writing to this
	tasksRunByName map[string]int

	taskName       string
	shouldTaskFail bool
	//task Duration in milliseconds
	taskCompleted     bool
	errorReturned     string
	shouldReturnError bool
}

func (event *mockTestTaskEvent) PerformTask(taskContext context.Context) (bool, error) {
	m.Lock()
	defer m.Unlock()

	if event.shouldReturnError {
		wg.Done()
		return false, errors.New(event.errorReturned)
	}

	time.Sleep(1 * time.Millisecond)
	event.taskCompleted = true

	// Keep track of the number of times the task is run, by name
	if event.tasksRunByName != nil && event.taskName != "" {
		event.tasksRunByName[event.taskName] = event.tasksRunByName[event.taskName] + 1
	}

	wg.Done()

	return event.shouldTaskFail, nil
}

type mockEmptyTask struct {
}

func (event *mockEmptyTask) PerformTask(taskContext context.Context) (bool, error) {
	return false, nil
}
