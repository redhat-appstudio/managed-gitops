package util

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type TaskRetryLoop struct {
	inputChan chan taskRetryLoopMessage

	// debugName is the name of the task retry loop, reported in the logs for debug purposes
	debugName string
}

type RetryableTask interface {
	// returns bool (whether or not the task should be retried), and error
	PerformTask(taskContext context.Context) (bool, error)
}

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

// func (loop *taskRetryLoop) removeTask(name string) {

// 	loop.inputChan <- taskRetryLoopMessage{
// 		msgType: taskRetryLoop_removeTask,
// 		payload: taskRetryMessage_removeTask{
// 			name: name,
// 		},
// 	}
// }

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
	payload interface{}
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

type waitingTaskEntry struct {
	name                   string
	task                   RetryableTask
	backoff                ExponentialBackoff
	nextScheduledRetryTime *time.Time
}

type internalTaskEntry struct {
	name         string
	task         RetryableTask
	backoff      ExponentialBackoff
	taskContext  context.Context
	cancelFunc   context.CancelFunc
	creationTime time.Time
}

const (
	ReportActiveTasksEveryXMinutes = 10 * time.Minute
)

func internalTaskRetryLoop(inputChan chan taskRetryLoopMessage, debugName string) {

	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("task-retry-name", debugName)

	// activeTaskMap is the set of tasks currently running in goroutines
	activeTaskMap := map[string]internalTaskEntry{}

	// tasks that are waiting to run. We ensure there are no duplicates in either list.
	waitingTasksByName := map[string]interface{}{}
	waitingTasks := []waitingTaskEntry{}

	const maxActiveRunners = 20

	nextReportActiveTasks := time.Now().Add(ReportActiveTasksEveryXMinutes)

	for {

		// Every X minutes, report how many tasks are in progress, and how many are waiting
		if time.Now().After(nextReportActiveTasks) {
			log.Info(fmt.Sprintf("task retry loop status [%v]: waitingTasks: %v, activeTasks: %v ", debugName, len(waitingTasks), len(activeTaskMap)))
			nextReportActiveTasks = time.Now().Add(ReportActiveTasksEveryXMinutes)
		}

		// Queue more running tasks if we have resources
		if len(waitingTasks) > 0 && len(activeTaskMap) < maxActiveRunners {

			updatedWaitingTasks := []waitingTaskEntry{}

			// TODO: GITOPSRVCE-68 - PERF - this is an inefficient algorithm for queuing tasks, because it causes an allocation and iteration through the entire list on every received event

			for idx := range waitingTasks {

				startTask := false

				task := waitingTasks[idx]

				if task.nextScheduledRetryTime == nil {
					startTask = true
				} else if time.Now().After(*task.nextScheduledRetryTime) {
					startTask = true
				}

				if startTask && len(activeTaskMap) < 20 {
					startNewTask(task, waitingTasksByName, activeTaskMap, inputChan, log)
				} else {
					updatedWaitingTasks = append(updatedWaitingTasks, task)
				}
			}

			// replace the waitingTask var, with a new list with started tasks removed
			waitingTasks = updatedWaitingTasks

		}

		msg := <-inputChan

		if msg.msgType == taskRetryLoop_addTask {

			addTaskMsg, ok := (msg.payload).(taskRetryMessage_addTask)
			if !ok {
				log.Error(nil, "SEVERE: unexpected message payload for addTask")
				continue
			}

			if _, exists := waitingTasksByName[addTaskMsg.name]; exists {
				log.V(LogLevel_Debug).Info("skipping message that is already in the wait queue: " + addTaskMsg.name)
				continue
			}

			waitingTasks = append(waitingTasks, waitingTaskEntry{
				name:    addTaskMsg.name,
				task:    addTaskMsg.task,
				backoff: addTaskMsg.backoff})

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

			log.V(LogLevel_Debug).Info("Removing task from retry loop: " + removeTaskMsg.name)
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

			// Now that the task is complete, remove it from the active map
			delete(activeTaskMap, workCompletedMsg.name)

			if workCompletedMsg.shouldRetry {
				log.V(LogLevel_Debug).Info("Adding failed task '" + taskEntry.name + "' to retry list")

				nextScheduledRetryTime := time.Now().Add(taskEntry.backoff.IncreaseAndReturnNewDuration())

				waitingTasks = append(waitingTasks, waitingTaskEntry{
					name:                   workCompletedMsg.name,
					task:                   taskEntry.task,
					nextScheduledRetryTime: &nextScheduledRetryTime,
					backoff:                taskEntry.backoff})
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

func startNewTask(taskToStart waitingTaskEntry, waitingTasksByName map[string]interface{}, activeTaskMap map[string]internalTaskEntry,
	inputChan chan taskRetryLoopMessage, log logr.Logger) {

	delete(waitingTasksByName, taskToStart.name)

	newTaskEntry := internalTaskEntry{
		name:         taskToStart.name,
		task:         taskToStart.task,
		backoff:      taskToStart.backoff,
		creationTime: time.Now(),
	}

	activeTaskMap[taskToStart.name] = newTaskEntry

	taskContext, taskCancelFunc := internalStartTaskRunner(&newTaskEntry, inputChan, log)
	newTaskEntry.taskContext = taskContext
	newTaskEntry.cancelFunc = taskCancelFunc

}

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
