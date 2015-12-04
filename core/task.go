/*
http://www.apache.org/licenses/LICENSE-2.0.txt


Copyright 2015 Intel Corporation

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package core

import (
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/intelsdi-x/snap/core/serror"
	"github.com/intelsdi-x/snap/pkg/schedule"
	"github.com/intelsdi-x/snap/scheduler/wmap"
)

// TaskState integer type
type TaskState int

// const task states
const (
	TaskDisabled TaskState = iota - 1
	TaskStopped
	TaskSpinning
	TaskFiring
	TaskEnded
	TaskStopping
)

var (
	// TaskStateLookup A map of task states
	TaskStateLookup = map[TaskState]string{
		TaskDisabled: "Disabled", // on error, not resumable
		TaskStopped:  "Stopped",  // stopped but resumable
		TaskSpinning: "Running",  // running
		TaskFiring:   "Running",  // running (firing can happen so briefly we don't want to try and render it as a string state)
		TaskEnded:    "Ended",    // ended, not resumable because the schedule will not fire again
		TaskStopping: "Stopping", // channel has been closed, wait for TaskStopped state
	}
)

// TaskWatcherCloser interface
type TaskWatcherCloser interface {
	Close() error
}

// TaskWatcherHandler interface defines
// task watch functions
type TaskWatcherHandler interface {
	CatchCollection([]Metric)
	CatchTaskStarted()
	CatchTaskStopped()
	CatchTaskDisabled(string)
}

// String returns task state
func (t TaskState) String() string {
	return TaskStateLookup[t]
}

// Task interface defines snap task
type Task interface {
	ID() string
	// Status() WorkflowState TODO, switch to string
	State() TaskState
	HitCount() uint
	GetName() string
	SetName(string)
	SetID(string)
	MissedCount() uint
	FailedCount() uint
	LastFailureMessage() string
	LastRunTime() *time.Time
	CreationTime() *time.Time
	DeadlineDuration() time.Duration
	SetDeadlineDuration(time.Duration)
	SetTaskID(id string)
	SetStopOnFailure(uint)
	GetStopOnFailure() uint
	Option(...TaskOption) TaskOption
	WMap() *wmap.WorkflowMap
	Schedule() schedule.Schedule
}

// TaskOption data type
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

// OptionStopOnFailure sets the tasks stopOnFailure
// The stopOnFailure is the number of consecutive task failures that will
// trigger disabling the task
func OptionStopOnFailure(v uint) TaskOption {
	return func(t Task) TaskOption {
		previous := t.GetStopOnFailure()
		t.SetStopOnFailure(v)
		log.WithFields(log.Fields{
			"_module":                   "core",
			"_block":                    "OptionStopOnFailure",
			"task-id":                   t.ID(),
			"task-name":                 t.GetName(),
			"consecutive failure limit": t.GetStopOnFailure(),
		}).Debug("Setting stop-on-failure limit for task")
		return OptionStopOnFailure(previous)
	}
}

// SetTaskName sets the name of the task.
// This is optional.
// If task name is not set, the task name is then defaulted to "Task-<task-id>"
func SetTaskName(name string) TaskOption {
	return func(t Task) TaskOption {
		previous := t.GetName()
		t.SetName(name)
		return SetTaskName(previous)
	}
}

// SetTaskID set task id
func SetTaskID(id string) TaskOption {
	return func(t Task) TaskOption {
		previous := t.ID()
		t.SetID(id)
		return SetTaskID(previous)
	}
}

// TaskErrors interface defines array of snap errors
type TaskErrors interface {
	Errors() []serror.SnapError
}
