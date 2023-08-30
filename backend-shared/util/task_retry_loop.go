package util

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	logutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/log"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// The goal of the task retry loop is to run multiple independent tasks concurrently, and to attempt tasks
// over and over again until they report success ("pass").
//
// Why would a task fail? Well it depends on what the task does, but primarily tasks in the GitOps service
// will retrieve some data from a database and/or from a K8s namespace, and then act on that data.
// So, for example, if the database went down, a task would fail to connect, and so the task would keep
// failing until the database came back up.
//
// Here is an example of a task:
//
// Imagine that we are implementing a task that deletes all the objects in a namespace.
//
// taskRetryLoop := NewTaskRetryLoop("(...)")
//
// deleteAllObjs := DeleteAllObjectsInNamespaceTask{}
//
// // 1) This will cause the 'DeleteAllObjects' task to start running, on namespace 'a'
// taskRetryLoop.AddTaskIfNotPresent("delete-namespace-A", deleteAllObjs, ...)
//
// // 2) This will cause the 'DeleteAllObjects' task to start running, on namespace 'b'
// taskRetryLoop.AddTaskIfNotPresent("delete-namespace-B", deleteAllObjs, ...)
//
// Both the tasks in step 1 and step 2 will run concurrently, because the task name is
// different ("delete-namespace-A" vs "delete-namespace-B"). If the task name was the
// same, only one task would be allowed to run concurrently.
//
// In order to run code as a task, the calling function must implement the 'RetryableTask' interface.
//
// The task retry loop supports multiple simultaneous tasks running at once. However, the task retry
// loop will not queue up the same 'task name' more than once. This de-duplication prevents the same
// task name from being queued more than once.
//
// Example: Using our same task example as above.
//
// deleteAllObjs := DeleteAllObjectsInNamespaceTask{}
//
// // 1) This will cause the 'DeleteAllObjects' task to start running, on namespace a
// taskRetryLoop.AddTaskIfNotPresent("delete-namespace-A", deleteAllObjs, ...)
//
// // 2) This will cause the 'DeleteAllObjects' task to start running, on namespace B
// taskRetryLoop.AddTaskIfNotPresent("delete-namespace-B", deleteAllObjs, ...)

// // 3) But what if AddTaskIfNotPresent is called on namespace A, before the task has finished with namespace A?
// taskRetryLoop.AddTaskIfNotPresent("delete-namespace-A", deleteAllObjs, ...)
//
// In this case, if 'delete-namespace-A' from step 1 has not started yet, then the task from step 3) will
// not run (it will be de-duplicated/ignored).
//
// However, if task 'delete-namespace-A' from step 1 has already started running, but not finished yet, then the task from step 3
// will NOT be de-duplicated: instead it will wait for the task from 1 to complete.
// - Tasks will only be de-duplicated from waitingTasks.
// - Because of this de-duplication, tasks submitted to the task retry loop must be idempotent.

type TaskRetryLoop struct {
	inputChan chan taskRetryLoopMessage

	// debugName is the name of the task retry loop, reported in the logs for debug purposes
	debugName string
}

// RetryableTask should be implemented for any task that wants to run in the task retry loop.
type RetryableTask interface {
	// Returns bool (true if the task should be retried, for example because it failed, false otherwise),
	// and error (an error to log on failure).
	//
	// NOTE: 'error' value does not affect whether the task will be retried, this error is only used for
	// error reporting.
	PerformTask(taskContext context.Context) (bool, error)
}

// AddTaskIfNotPresent will queue a task to run within the task retry loop
func (loop *TaskRetryLoop) AddTaskIfNotPresent(name string, task RetryableTask, backoff ExponentialBackoff) {

	loop.inputChan <- taskRetryLoopMessage{
		msgType: taskRetryLoop_addTask,
		payload: taskRetryMessage_addTask{
			name:    name,
			task:    task,
			backoff: backoff,
		},
	}
}

type taskRetryMessageType string

const (
	taskRetryLoop_addTask       taskRetryMessageType = "addTask"
	taskRetryLoop_removeTask    taskRetryMessageType = "removeTask"
	taskRetryLoop_workCompleted taskRetryMessageType = "workCompleted"
	taskRetryLoop_tick          taskRetryMessageType = "tick"
)

const (
	minimumEventTick = time.Duration(time.Millisecond * 200)
)

type taskRetryLoopMessage struct {
	msgType taskRetryMessageType
	payload any
}

type taskRetryMessage_addTask struct {
	name    string
	backoff ExponentialBackoff
	task    RetryableTask
}
type taskRetryMessage_removeTask struct {
	name string
}

type taskRetryMessage_workCompleted struct {
	name        string
	shouldRetry bool
	resultErr   error
}

func NewTaskRetryLoop(debugName string) (loop *TaskRetryLoop) {

	res := &TaskRetryLoop{
		inputChan: make(chan taskRetryLoopMessage),
		debugName: debugName,
	}

	go internalTaskRetryLoop(res.inputChan, res.debugName)

	// Ensure the message queue logic runs at least every 200 msecs
	go func() {
		ticker := time.NewTicker(minimumEventTick)
		for {
			<-ticker.C
			res.inputChan <- taskRetryLoopMessage{
				msgType: taskRetryLoop_tick,
				payload: nil,
			}
		}
		// TODO: GITOPSRVCE-68 - PERF - I'm sure a more complex form of this logic could calculate the length of time until the next task is 'due'.
	}()

	return res
}

// waitingTaskContainer contains all waiting tasks
// - waitingTasksByName and waitingTasks contain the same test of tasks, just organized in different collections
type waitingTaskContainer struct {

	// waitingTasksByName is a map of tasks, from the name of the task -> the task itself
	// - used to tell if a task is already present in the waiting tasks list
	waitingTasksByName map[string]any

	// waitingTasks is an ordered list of tasks, ordered in the order in which they were received
	// - used to tell which task should run next
	waitingTasks []waitingTaskEntry
}

// waitingTaskEntry represents a single waiting task
type waitingTaskEntry struct {
	name                   string
	task                   RetryableTask
	backoff                ExponentialBackoff
	nextScheduledRetryTime *time.Time
}

func (wte *waitingTaskContainer) isWorkAvailable() bool {
	return len(wte.waitingTasks) > 0
}

func (wte *waitingTaskContainer) addTask(entry waitingTaskEntry, log logr.Logger) {

	// Check if the task already exists in the list (by name)
	if _, exists := wte.waitingTasksByName[entry.name]; exists {
		log.V(logutil.LogLevel_Debug).Info("skipping duplicate task in addTask", "taskName", entry.name)
		return
	}

	// Otherwise, add the task
	wte.waitingTasks = append(wte.waitingTasks, entry)
	wte.waitingTasksByName[entry.name] = entry
}

// internalTaskEntry represents a single active (currently running) task
type internalTaskEntry struct {
	name         string
	task         RetryableTask
	backoff      ExponentialBackoff
	taskContext  context.Context
	cancelFunc   context.CancelFunc
	creationTime time.Time
}

const (
	// ReportActiveTasksEveryXMinutes is a ticker interval, which will output a status on how many
	// tasks are active/waiting. This is useful for ensuring the service is working as expected.
	ReportActiveTasksEveryXMinutes = 10 * time.Minute
)

func internalTaskRetryLoop(inputChan chan taskRetryLoopMessage, debugName string) {

	ctx := context.Background()
	log := log.FromContext(ctx).WithName("task-retry-loop").WithValues("task-retry-name", debugName)

	// activeTaskMap is the set of tasks currently running in goroutines
	activeTaskMap := map[string]internalTaskEntry{}

	// tasks that are waiting to run. We ensure there are no duplicates in either list.
	waitingTaskContainer := waitingTaskContainer{
		waitingTasksByName: map[string]any{},
		waitingTasks:       []waitingTaskEntry{},
	}

	const maxActiveRunners = 20

	nextReportActiveTasks := time.Now().Add(ReportActiveTasksEveryXMinutes)

	for {

		// Every X minutes, report how many tasks are in progress, and how many are waiting. This allows us
		// to identify bottenecks in task retry queues.
		if time.Now().After(nextReportActiveTasks) {
			log.Info(fmt.Sprintf("task retry loop status [%v]: waitingTasks: %v, activeTasks: %v ", debugName,
				len(waitingTaskContainer.waitingTasks), len(activeTaskMap)))

			nextReportActiveTasks = time.Now().Add(ReportActiveTasksEveryXMinutes)
		}

		// Queue more running tasks if we have resources
		if waitingTaskContainer.isWorkAvailable() && len(activeTaskMap) < maxActiveRunners {

			updatedWaitingTasks := []waitingTaskEntry{}

			// TODO: GITOPSRVCE-68 - PERF - this is an inefficient algorithm for queuing tasks, because it causes an allocation and iteration through the entire list on every received event

			for idx := range waitingTaskContainer.waitingTasks {

				task := waitingTaskContainer.waitingTasks[idx]

				startTask := false
				{
					if task.nextScheduledRetryTime == nil {
						startTask = true
					} else if time.Now().After(*task.nextScheduledRetryTime) {
						startTask = true
					}

					// Don't start a task (yet) if it's already running
					if _, exists := activeTaskMap[task.name]; exists {
						startTask = false
					}
				}

				// If the task is ready to go, and we still have room for it, start it
				if startTask && len(activeTaskMap) < maxActiveRunners {

					prevActiveTaskMapSize := len(activeTaskMap) // used for sanity tests
					prevWaitingTasksByNameSize := len(waitingTaskContainer.waitingTasksByName)

					startNewTask(task, &waitingTaskContainer, activeTaskMap, inputChan, log)

					// Sanity check the task start
					if len(activeTaskMap) != prevActiveTaskMapSize+1 {
						log.Error(nil, "SEVERE: active task map did not grow after startNewTask was called")
					}
					if len(waitingTaskContainer.waitingTasksByName) != prevWaitingTasksByNameSize-1 {
						log.Error(nil, "SEVERE: waiting tasks by name did not shrink after startNewTask was called")
					}

				} else {
					// Otherwise, we don't yet have room for it, or it isn't ready yet, so keep it in the list
					// of waiting tasks.
					updatedWaitingTasks = append(updatedWaitingTasks, task)
				}
			}

			// replace the waitingTask var, with a new list with started tasks removed
			waitingTaskContainer.waitingTasks = updatedWaitingTasks

		}

		// After we have ensured our task queue is full, pull the next message from the channel.

		msg := <-inputChan

		if msg.msgType == taskRetryLoop_addTask {

			log.V(logutil.LogLevel_Debug).Info("Task retry loop: addTask received", "msg", msg)

			addTaskMsg, ok := (msg.payload).(taskRetryMessage_addTask)
			if !ok {
				log.Error(nil, "SEVERE: unexpected message payload for addTask")
				continue
			}

			newWaitingTaskEntry := waitingTaskEntry{
				name:    addTaskMsg.name,
				task:    addTaskMsg.task,
				backoff: addTaskMsg.backoff}

			waitingTaskContainer.addTask(newWaitingTaskEntry, log)

		} else if msg.msgType == taskRetryLoop_removeTask {

			// NOTE: this only removes from the active task map

			removeTaskMsg, ok := (msg.payload).(taskRetryMessage_removeTask)
			if !ok {
				log.Error(nil, "SEVERE: unexpected message payload for removeTask")
				continue
			}

			taskEntry, ok := activeTaskMap[removeTaskMsg.name]
			if !ok {
				log.Error(nil, "task not found in map: "+removeTaskMsg.name)
				continue
			}

			log.V(logutil.LogLevel_Debug).Info("Removing task from retry loop: " + removeTaskMsg.name)
			delete(activeTaskMap, removeTaskMsg.name)

			if taskEntry.cancelFunc != nil {
				go taskEntry.cancelFunc()
			}

		} else if msg.msgType == taskRetryLoop_workCompleted {

			workCompletedMsg, ok := (msg.payload).(taskRetryMessage_workCompleted)
			if !ok {
				log.Error(nil, "SEVERE: unexpected message payload for workCompleted")
				continue
			}

			taskEntry, ok := activeTaskMap[workCompletedMsg.name]
			if !ok {
				log.Error(nil, "task not found in map: "+workCompletedMsg.name)
				continue
			}

			if taskEntry.name != workCompletedMsg.name { // sanity test
				log.Error(nil, "SEVERE: taskEntry name should match work completed msg name")
				continue
			}

			// Now that the task is complete, remove it from the active map
			delete(activeTaskMap, workCompletedMsg.name)

			log.V(logutil.LogLevel_Debug).Info("Task retry loop: task completed '"+taskEntry.name+"'", "shouldRetry", workCompletedMsg.shouldRetry)

			if workCompletedMsg.shouldRetry {
				log.V(logutil.LogLevel_Debug).Info("Adding failed task '" + taskEntry.name + "' to retry list")

				nextScheduledRetryTime := time.Now().Add(taskEntry.backoff.IncreaseAndReturnNewDuration())

				waitingTaskEntry := waitingTaskEntry{
					name:                   taskEntry.name,
					task:                   taskEntry.task,
					nextScheduledRetryTime: &nextScheduledRetryTime,
					backoff:                taskEntry.backoff}

				waitingTaskContainer.addTask(waitingTaskEntry, log)
			}
			continue

		} else if msg.msgType == taskRetryLoop_tick {
			// no processing required.
			continue
		} else {
			log.Error(nil, "SEVERE: unexpected message type: "+string(msg.msgType))
			continue
		}
	}
}

func startNewTask(taskToStart waitingTaskEntry, waitingTaskContainer *waitingTaskContainer, activeTaskMap map[string]internalTaskEntry,
	inputChan chan taskRetryLoopMessage, log logr.Logger) {

	taskName := taskToStart.name

	delete(waitingTaskContainer.waitingTasksByName, taskName)

	newTaskEntry := internalTaskEntry{
		name:         taskName,
		task:         taskToStart.task,
		backoff:      taskToStart.backoff,
		creationTime: time.Now(),
	}

	activeTaskMap[taskName] = newTaskEntry

	taskContext, taskCancelFunc := internalStartTaskRunner(&newTaskEntry, inputChan, log)
	newTaskEntry.taskContext = taskContext
	newTaskEntry.cancelFunc = taskCancelFunc

}

// internalStartTaskRunner starts a new goroutine that is responsible for running the given task, and then returning the result to internalTaskRetryLoop
func internalStartTaskRunner(taskEntry *internalTaskEntry, workComplete chan taskRetryLoopMessage, log logr.Logger) (context.Context, context.CancelFunc) {

	taskContext, cancelFunc := context.WithCancel(context.Background())

	go func() {

		var shouldRetry bool
		var resultErr error

		isPanic, panicErr := CatchPanic(func() error {
			shouldRetry, resultErr = taskEntry.task.PerformTask(taskContext)
			return nil
		})

		if isPanic {
			resultErr = panicErr
		}

		if resultErr != nil {
			log.Error(resultErr, "internalStartTaskRunner error for "+taskEntry.name, "shouldRetry", shouldRetry)
		}

		workComplete <- taskRetryLoopMessage{
			msgType: taskRetryLoop_workCompleted,
			payload: taskRetryMessage_workCompleted{
				name:        taskEntry.name,
				shouldRetry: shouldRetry,
				resultErr:   resultErr,
			},
		}

	}()

	return taskContext, cancelFunc
}
