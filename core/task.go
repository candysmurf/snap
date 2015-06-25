package core

import "time"

type TaskState int

const (
	TaskDisabled TaskState = iota - 1
	TaskStopped
	TaskSpinning
	TaskFiring
)

var (
	TaskStateLookup = map[TaskState]string{
		TaskDisabled: "Disabled",
		TaskStopped:  "Stopped",
		TaskSpinning: "Spinning",
		TaskFiring:   "Firing",
	}
)

func (t TaskState) String() string {
	return TaskStateLookup[t]
}

type Task interface {
	ID() uint64
	// Status() WorkflowState TODO, switch to string
	State() TaskState
	HitCount() uint
	GetName() string
	SetName(string)
	MissedCount() uint
	FailedCount() uint
	LastFailureMessage() string
	LastRunTime() *time.Time
	CreationTime() *time.Time
	DeadlineDuration() time.Duration
	SetDeadlineDuration(time.Duration)
	SetStopOnFailure(uint)
	GetStopOnFailure() uint
	Option(...TaskOption) TaskOption
}

type TaskOption func(Task) TaskOption

// TaskDeadlineDuration sets the tasks deadline.
// The deadline is the amount of time that can pass before a worker begins
// processing the tasks collect job.
func TaskDeadlineDuration(v time.Duration) TaskOption {
	return func(t Task) TaskOption {
		previous := t.DeadlineDuration()
		t.SetDeadlineDuration(v)
		return TaskDeadlineDuration(previous)
	}
}

// TaskStopOnFailure sets the tasks stopOnFailure
// The stopOnFailure is the number of consecutive task failures that will
// trigger disabling the task
func OptionStopOnFailure(v uint) TaskOption {
	return func(t Task) TaskOption {
		previous := t.GetStopOnFailure()
		t.SetStopOnFailure(v)
		return OptionStopOnFailure(previous)
	}
}

//SetTaskName sets the name of the task.
//This is optional.
//If task name is not set, the task name is then defaulted to "Task-<task-id>"
func SetTaskName(name string) TaskOption {
	return func(t Task) TaskOption {
		previous := t.GetName()
		t.SetName(name)
		return SetTaskName(previous)
	}
}

type TaskErrors interface {
	Errors() []error
}
