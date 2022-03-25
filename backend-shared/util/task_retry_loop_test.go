package util

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"math/rand"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
	"sync"
	"time"
)

var (
	wg sync.WaitGroup
	m sync.Mutex
)

const (
	numberOfTasks int = 5000
)

type TestEvent struct {
	shouldTaskFail bool
	//task Duration in milliseconds
	taskDuration  int
	taskCompleted bool
	errorReturned string
	shouldPanic   bool
}

func (event *TestEvent) PerformTask(taskContext context.Context) (bool, error) {
	if event.shouldPanic {
		wg.Done()
		panic(event.errorReturned)
	}

	m.Lock()

	time.Sleep(time.Duration(event.taskDuration) * time.Millisecond)
	event.taskCompleted = true
	
	m.Unlock()

	wg.Done()

	return event.shouldTaskFail, nil
}

var _ = Describe("Task Retry Loop Unit Tests", func() {

	var (
		workComplete chan taskRetryLoopMessage
		ctx          context.Context
		log          logr.Logger
	)

	BeforeEach(func() {
		workComplete = make(chan taskRetryLoopMessage)
		ctx = context.Background()
		log = logger.FromContext(ctx)
	})

	var _ = Describe("AddTaskIfNotPresent Test", func() {

		Context("AddTaskIfNotPresent Test", func() {

			It("should generate 5000 tasks with random IDs and execute them successfully", func() {

				testEvent := &TestEvent{shouldTaskFail: false, taskDuration: 10}
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
	})

	var _ = Describe("AddTaskIfNotPresent Test", func() {

		Context("Test De-duplication", func() {

			It("should generate 5000 tasks with randomly selected among a list of 5 names, and the number of active tasks doesn't exceed the size of the list", func() {

				testEvent := &TestEvent{shouldTaskFail: false, taskDuration: 5000}
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
	})

	var _ = Describe("startNewTask Test", func() {

		Context("startNewTask Test", func() {

			It("ensures that calling startTask removes the task from 'waitingTasksByName'", func() {

				activeTaskMap := make(map[string]internalTaskEntry)
				taskToStart := waitingTaskEntry{name: "test-task"}
				waitingTasksByName := make(map[string]interface{})

				waitingTasksByName["test-task"] = waitingTaskEntry{}

				Expect(len(waitingTasksByName)).To(Equal(1))

				startNewTask(taskToStart, waitingTasksByName, activeTaskMap, workComplete, log)

				Expect(len(waitingTasksByName)).To(Equal(0))
			})
		})
	})

	var _ = Describe("internalTaskRunner Test", func() {

		Context("internalTaskRunner Test", func() {

			It("ensures that the task provided runs as expected", func() {

				workComplete := make(chan taskRetryLoopMessage)
				task := &TestEvent{shouldTaskFail: false, taskDuration: 100}
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
		})
	})

	var _ = Describe("internalTaskRunner Test", func() {

		Context("internalTaskRunner Test", func() {

			It("ensures that when the task returns an error, it is communicated to the channel in a 'taskRetryLoopMessage'", func() {

				task := &TestEvent{shouldTaskFail: true, taskDuration: 100, errorReturned: "internalTaskRunner error", shouldPanic: true}
				taskEntry := &internalTaskEntry{task: task, name: "test-task", creationTime: time.Now()}

				wg.Add(1)
				internalStartTaskRunner(taskEntry, workComplete, log)
				wg.Wait()

				receivedMsg := <-workComplete

				workCompletedMsg, _ := (receivedMsg.payload).(taskRetryMessage_workCompleted)

				Expect(workCompletedMsg.resultErr).To(MatchError(fmt.Errorf("panic: internalTaskRunner error")))
			})
		})
	})

	var _ = Describe("internalTaskRunner Test", func() {

		Context("internalTaskRunner Test", func() {

			It("ensure that when the task return true for retry, it is communicated to the channel in a 'taskRetryLoopMessage'", func() {

				task := &TestEvent{shouldTaskFail: true, taskDuration: 100, shouldPanic: false}
				taskEntry := &internalTaskEntry{task: task, name: "test-task", creationTime: time.Now()}

				wg.Add(1)
				internalStartTaskRunner(taskEntry, workComplete, log)
				wg.Wait()

				receivedMsg := <-workComplete

				workCompletedMsg, _ := (receivedMsg.payload).(taskRetryMessage_workCompleted)

				Expect(workCompletedMsg.shouldRetry).To(BeTrue())
			})
		})
	})

	var _ = Describe("internalTaskRunner Test", func() {

		Context("internalTaskRunner Test", func() {

			It("ensure that when the task return false for retry, it is communicated to the channel in a 'taskRetryLoopMessage'", func() {

				task := &TestEvent{shouldTaskFail: false, taskDuration: 100, shouldPanic: false}
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

})
