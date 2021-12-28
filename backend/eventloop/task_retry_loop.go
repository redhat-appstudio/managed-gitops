package eventloop

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend/util"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type taskRetryLoop struct {
	inputChan chan taskRetryLoopMessage
}

type retryableTask interface {
	// returns bool (whether or not the task should be retried), and error
	performTask(taskContext context.Context) (bool, error)
}

func (loop *taskRetryLoop) addTaskIfNotPresent(name string, task retryableTask, backoff util.ExponentialBackoff) {

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

type taskRetryLoopMessage struct {
	msgType taskRetryMessageType
	payload interface{}
}

type taskRetryMessage_addTask struct {
	name    string
	backoff util.ExponentialBackoff
	task    retryableTask
}
type taskRetryMessage_removeTask struct {
	name string
}

type taskRetryMessage_workCompleted struct {
	name        string
	shouldRetry bool
	resultErr   error
}

func newTaskRetryLoop() (loop *taskRetryLoop) {

	res := &taskRetryLoop{
		inputChan: make(chan taskRetryLoopMessage),
	}

	go internalTaskRetryLoop(res.inputChan)

	// Ensure the message queue logic runs at least every 200 msecs
	go func() {
		ticker := time.NewTicker(time.Millisecond * 200) // TODO: move to constant
		for {
			<-ticker.C
			res.inputChan <- taskRetryLoopMessage{
				msgType: taskRetryLoop_tick,
				payload: nil,
			}
		}
		// TODO: PERF - I'm sure a more complex form of this logic could calculate the length of time until the next task is 'due'.
	}()

	return res
}

type waitingTaskEntry struct {
	name                   string
	task                   retryableTask
	backoff                util.ExponentialBackoff
	nextScheduledRetryTime *time.Time
}

type internalTaskEntry struct {
	name        string
	task        retryableTask
	backoff     util.ExponentialBackoff
	taskContext context.Context
	cancelFunc  context.CancelFunc
}

func internalTaskRetryLoop(inputChan chan taskRetryLoopMessage) {

	ctx := context.Background()
	log := log.FromContext(ctx)

	// activeTaskMap is the set of tasks currently running in goroutines
	activeTaskMap := map[string]internalTaskEntry{}

	// tasks that are waiting to run. We ensure there are no duplicates in either list.
	waitingTasksByName := map[string]interface{}{}
	waitingTasks := []waitingTaskEntry{}

	const maxActiveRunners = 20

	for {

		// TODO: DEBT - Add periodic logging of active tasks, waiting tasks, here.

		// Queue more running tasks if we have resources
		if len(waitingTasks) > 0 && len(activeTaskMap) < maxActiveRunners {

			updatedWaitingTasks := []waitingTaskEntry{}

			// TODO: PERF - this is an inefficient algorithm for queuing tasks, because it causes an allocation and iteration through the entire list on every received event

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
				log.V(sharedutil.LogLevel_Debug).Info("skipping message that is already in the wait queue: " + addTaskMsg.name)
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

			log.V(sharedutil.LogLevel_Debug).Info("Removing task from retry loop: " + removeTaskMsg.name)
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
				log.V(sharedutil.LogLevel_Debug).Info("Adding failed task '" + taskEntry.name + "' to retry list")

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
		name:    taskToStart.name,
		task:    taskToStart.task,
		backoff: taskToStart.backoff}

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

		isPanic, panicErr := sharedutil.CatchPanic(func() error {
			shouldRetry, resultErr = taskEntry.task.performTask(taskContext)
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
