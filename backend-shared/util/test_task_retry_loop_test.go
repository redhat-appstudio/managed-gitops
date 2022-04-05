package util

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	wg sync.WaitGroup
	m  sync.Mutex
)

var _ = Describe("Task Retry Loop Unit Tests", func() {

	const (
		numberOfTasks int = 5000
	)

	var (
		workComplete chan taskRetryLoopMessage
		ctx          context.Context
		log          logr.Logger
	)

	BeforeEach(func() {
		workComplete = make(chan taskRetryLoopMessage)
		ctx = context.Background()
		log = logger.FromContext(ctx)
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

		It("should generate 5000 tasks with random IDs and execute them successfully", func() {

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

		It("should generate 5000 tasks with randomly selected among a list of 5 names, and the number of active tasks doesn't exceed the size of the list", func() {

			testEvent := &mockTestTaskEvent{shouldTaskFail: false}
			taskRetryLoop := NewTaskRetryLoop("dummy-name")
			taskNames := [5]string{"a", "b", "c", "d", "e"}

			for i := 0; i < numberOfTasks; i++ {
				wg.Add(1)
				taskName := taskNames[rand.Intn(len(taskNames))]
				taskRetryLoop.AddTaskIfNotPresent(taskName, testEvent, ExponentialBackoff{Factor: 2, Min: time.Duration(100 * time.Microsecond), Max: time.Duration(1 * time.Second), Jitter: true})
			}

			fmt.Printf("Waiting for %d tasks to complete...\n", numberOfTasks)

			wg.Wait()

			fmt.Printf("%d tasks successfully completed\n", numberOfTasks)
		})
	})

	Context("startNewTask Test", func() {

		It("ensures that calling startTask removes the task from 'waitingTasksByName'", func() {

			task := &mockEmptyTask{}

			activeTaskMap := make(map[string]internalTaskEntry)
			taskToStart := waitingTaskEntry{name: "test-task", task: task}
			waitingTasksByName := make(map[string]interface{})

			waitingTasksByName["test-task"] = waitingTaskEntry{}

			Expect(len(waitingTasksByName)).To(Equal(1))

			startNewTask(taskToStart, waitingTasksByName, activeTaskMap, workComplete, log)

			Expect(len(waitingTasksByName)).To(Equal(0))
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
			Expect(workCompletedMsg.resultErr).To(BeNil())
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
	shouldTaskFail bool
	//task Duration in milliseconds
	taskCompleted     bool
	errorReturned     string
	shouldReturnError bool
}

func (event *mockTestTaskEvent) PerformTask(taskContext context.Context) (bool, error) {
	if event.shouldReturnError {
		wg.Done()
		return false, fmt.Errorf(event.errorReturned)
	}

	m.Lock()

	time.Sleep(1 * time.Millisecond)
	event.taskCompleted = true

	m.Unlock()

	wg.Done()

	return event.shouldTaskFail, nil
}

type mockEmptyTask struct {
}

func (event *mockEmptyTask) PerformTask(taskContext context.Context) (bool, error) {
	return false, nil
}
