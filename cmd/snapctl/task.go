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

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/codegangsta/cli"
	"github.com/intelsdi-x/snap/mgmt/rest/client"
	"github.com/intelsdi-x/snap/scheduler/wmap"

	"github.com/ghodss/yaml"
)

var (
	// padding to picking a time to start a "NOW" task
	createTaskNowPad = time.Second * 1
	timeParseFormat  = "3:04PM"
	dateParseFormat  = "1-02-2006"
	unionParseFormat = timeParseFormat + " " + dateParseFormat
)

// const defines constants for the length of a string.
const (
	// K defines the length of a string.
	K = 1000
	M = 1000 * K
	G = 1000 * M
	T = 1000 * G
	P = 1000 * T
)

func trunc(n int) string {
	var u string

	switch {
	case n >= P:
		u = "P"
		n /= P
	case n >= T:
		u = "T"
		n /= T
	case n >= G:
		u = "G"
		n /= G
	case n >= M:
		u = "M"
		n /= M
	case n >= K:
		u = "K"
		n /= K
	default:
		return strconv.Itoa(n)
	}
	return strconv.Itoa(n) + u
}

type task struct {
	Version  int
	Schedule *client.Schedule
	Workflow *wmap.WorkflowMap
	Name     string
}

func createTask(ctx *cli.Context) {
	if ctx.IsSet("task-manifest") {
		fmt.Println("Using task manifest to create task")
		createTaskUsingTaskManifest(ctx)
	} else if ctx.IsSet("workflow-manifest") {
		fmt.Println("Using workflow manifest to create task")
		createTaskUsingWFManifest(ctx)
	} else {
		fmt.Println("Must provide either --task-manifest or --workflow-manifest arguments")
		cli.ShowCommandHelp(ctx, ctx.Command.Name)
		os.Exit(1)
	}

	os.Exit(0)

}

func createTaskUsingTaskManifest(ctx *cli.Context) {
	path := ctx.String("task-manifest")
	ext := filepath.Ext(path)
	file, e := ioutil.ReadFile(path)
	if e != nil {
		fmt.Printf("File error [%s]- %v\n", ext, e)
		os.Exit(1)
	}

	t := task{}
	switch ext {
	case ".yaml", ".yml":
		e = yaml.Unmarshal(file, &t)
		if e != nil {
			fmt.Printf("Error parsing YAML file input - %v\n", e)
			os.Exit(1)
		}
	case ".json":
		e = json.Unmarshal(file, &t)
		if e != nil {
			fmt.Printf("Error parsing JSON file input - %v\n", e)
			os.Exit(1)
		}
	default:
		fmt.Printf("Unsupported file type %s\n", ext)
		os.Exit(1)
	}

	t.Name = ctx.String("name")
	if t.Version != 1 {
		fmt.Println("Invalid version provided")
		os.Exit(1)
	}
	r := pClient.CreateTask(t.Schedule, t.Workflow, t.Name, !ctx.IsSet("no-start"))

	if r.Err != nil {
		errors := strings.Split(r.Err.Error(), " -- ")
		fmt.Println("Error creating task:")
		for _, err := range errors {
			fmt.Printf("%v\n", err)
		}
		os.Exit(1)
	}
	fmt.Println("Task created")
	fmt.Printf("ID: %s\n", r.ID)
	fmt.Printf("Name: %s\n", r.Name)
	fmt.Printf("State: %s\n", r.State)
}

func createTaskUsingWFManifest(ctx *cli.Context) {
	// Get the workflow
	path := ctx.String("workflow-manifest")
	ext := filepath.Ext(path)
	file, e := ioutil.ReadFile(path)
	if e != nil {
		fmt.Printf("File error [%s]- %v\n", ext, e)
		os.Exit(1)
	}

	var wf *wmap.WorkflowMap
	switch ext {
	case ".yaml", ".yml":
		// e = yaml.Unmarshal(file, &t)
		wf, e = wmap.FromYaml(file)
		if e != nil {
			fmt.Printf("Error parsing YAML file input - %v\n", e)
			os.Exit(1)
		}
	case ".json":
		wf, e = wmap.FromJson(file)
		// e = json.Unmarshal(file, &t)
		if e != nil {
			fmt.Printf("Error parsing JSON file input - %v\n", e)
			os.Exit(1)
		}
	}
	// Get the task name
	name := ctx.String("name")

	// Get the interval
	i := ctx.String("interval")
	_, err := time.ParseDuration(i)
	if err != nil {
		fmt.Printf("Bad interval format:\n%v\n", err)
		os.Exit(1)
	}

	var sch *client.Schedule
	// None of these mean it is a simple schedule
	if !ctx.IsSet("start-date") && !ctx.IsSet("start-time") && !ctx.IsSet("stop-date") && !ctx.IsSet("stop-time") {
		// Check if duration was set
		if ctx.IsSet("duration") {
			d, err := time.ParseDuration(ctx.String("duration"))
			if err != nil {
				fmt.Printf("Bad duration format:\n%v\n", err)
				os.Exit(1)
			}
			start := time.Now().Add(createTaskNowPad)
			stop := start.Add(d)
			sch = &client.Schedule{
				Type:      "windowed",
				Interval:  i,
				StartTime: &start,
				StopTime:  &stop,
			}
		} else {
			// No start or stop and no duration == simple schedule
			sch = &client.Schedule{
				Type:     "simple",
				Interval: i,
			}
		}
	} else {
		// We have some form of windowed schedule
		start := mergeDateTime(
			strings.ToUpper(ctx.String("start-time")),
			strings.ToUpper(ctx.String("start-date")),
		)
		stop := mergeDateTime(
			strings.ToUpper(ctx.String("stop-time")),
			strings.ToUpper(ctx.String("stop-date")),
		)

		// Use duration to create missing start or stop
		if ctx.IsSet("duration") {
			d, err := time.ParseDuration(ctx.String("duration"))
			if err != nil {
				fmt.Printf("Bad duration format:\n%v\n", err)
				os.Exit(1)
			}
			// if start is set and stop is not then use duration to create stop
			if start != nil && stop == nil {
				t := start.Add(d)
				stop = &t
			}
			// if stop is set and start is not then use duration to create start
			if stop != nil && start == nil {
				t := stop.Add(d * -1)
				start = &t
			}
		}
		sch = &client.Schedule{
			Type:      "windowed",
			Interval:  i,
			StartTime: start,
			StopTime:  stop,
		}
	}
	// Create task
	r := pClient.CreateTask(sch, wf, name, !ctx.IsSet("no-start"))
	if r.Err != nil {
		errors := strings.Split(r.Err.Error(), " -- ")
		fmt.Println("Error creating task:")
		for _, err := range errors {
			fmt.Printf("%v\n", err)
		}
		os.Exit(1)
	}
	fmt.Println("Task created")
	fmt.Printf("ID: %s\n", r.ID)
	fmt.Printf("Name: %s\n", r.Name)
	fmt.Printf("State: %s\n", r.State)
}

func mergeDateTime(tm, dt string) *time.Time {
	reTm := time.Now().Add(createTaskNowPad)
	if dt == "" && tm == "" {
		return nil
	}
	if dt != "" {
		t, err := time.Parse(dateParseFormat, dt)
		if err != nil {
			fmt.Printf("Error creating task:\n%v\n", err)
			os.Exit(1)
		}
		reTm = t
	}

	if tm != "" {
		_, err := time.ParseInLocation(timeParseFormat, tm, time.Local)
		if err != nil {
			fmt.Printf("Error creating task:\n%v\n", err)
			os.Exit(1)
		}
		reTm, err = time.ParseInLocation(unionParseFormat, fmt.Sprintf("%s %s", tm, reTm.Format(dateParseFormat)), time.Local)
		if err != nil {
			fmt.Printf("Error creating task:\n%v\n", err)
			os.Exit(1)
		}
	}
	return &reTm
}

func listTask(ctx *cli.Context) {
	tasks := pClient.GetTasks()
	if tasks.Err != nil {
		fmt.Printf("Error getting tasks:\n%v\n", tasks.Err)
		os.Exit(1)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', 0)
	printFields(w, false, 0,
		"ID",
		"NAME",
		"STATE",
		"HIT",
		"MISS",
		"FAIL",
		"CREATED",
		"LAST FAILURE",
	)
	for _, task := range tasks.ScheduledTasks {
		printFields(w, false, 0,
			task.ID,
			task.Name,
			task.State,
			trunc(task.HitCount),
			trunc(task.MissCount),
			trunc(task.FailedCount),
			task.CreationTime().Format(unionParseFormat),
			task.LastFailureMessage,
		)
	}
	w.Flush()
}

func watchTask(ctx *cli.Context) {
	if len(ctx.Args()) != 1 {
		fmt.Print("Incorrect usage\n")
		cli.ShowCommandHelp(ctx, ctx.Command.Name)
		os.Exit(1)
	}

	id := ctx.Args().First()
	r := pClient.WatchTask(id)
	if r.Err != nil {
		fmt.Println(r.Err)
		os.Exit(1)
	}
	fmt.Printf("Watching Task (%s):\n", id)

	// catch interrupt so we signal the server we are done before exiting
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	var lines int
	go func() {
		<-c
		fmt.Printf("%sStopping task watch\n", strings.Repeat("\n", lines))
		r.Close()
		return
	}()

	w := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', 0)
	printFields(w, false, 0, "NAMESPACE", "DATA", "TIMESTAMP", "SOURCE")
	// Loop listening to events
	for {
		select {
		case e := <-r.EventChan:
			switch e.EventType {
			case "metric-event":
				sort.Sort(e.Event)
				for _, event := range e.Event {
					printFields(w, false, 0,
						event.Namespace,
						event.Data,
						event.Timestamp,
						event.Source,
					)
				}
				lines = len(e.Event)
				fmt.Fprintf(w, "\033[%dA\n", lines+1)
				w.Flush()
			default:
				fmt.Printf("%s[%s]\n", strings.Repeat("\n", lines), e.EventType)
			}

		case <-r.DoneChan:
			return
		}
	}

}

func startTask(ctx *cli.Context) {
	if len(ctx.Args()) != 1 {
		fmt.Print("Incorrect usage\n")
		cli.ShowCommandHelp(ctx, ctx.Command.Name)
		os.Exit(1)
	}

	id := ctx.Args().First()
	r := pClient.StartTask(id)
	if r.Err != nil {
		if strings.Contains(r.Err.Error(), "Task is already running.") {
			fmt.Println("Task is already running")
			fmt.Printf("ID: %s\n", id)
			os.Exit(0)
		}
		fmt.Printf("Error starting task:\n%v\n", r.Err)
		os.Exit(1)
	}
	fmt.Println("Task started:")
	fmt.Printf("ID: %s\n", r.ID)
}

func stopTask(ctx *cli.Context) {
	if len(ctx.Args()) != 1 {
		fmt.Print("Incorrect usage\n")
		cli.ShowCommandHelp(ctx, ctx.Command.Name)
		os.Exit(1)
	}

	id := ctx.Args().First()
	r := pClient.StopTask(id)
	if r.Err != nil {
		if strings.Contains(r.Err.Error(), "Task is already stopped.") {
			fmt.Println("Task is already stopped")
			fmt.Printf("ID: %s\n", id)
			os.Exit(0)
		}
		fmt.Printf("Error stopping task:\n%v\n", r.Err)
		os.Exit(1)
	}
	fmt.Println("Task stopped:")
	fmt.Printf("ID: %s\n", r.ID)
}

func removeTask(ctx *cli.Context) {
	if len(ctx.Args()) != 1 {
		fmt.Print("Incorrect usage\n")
		cli.ShowCommandHelp(ctx, ctx.Command.Name)
		os.Exit(1)
	}

	id := ctx.Args().First()
	r := pClient.RemoveTask(id)
	if r.Err != nil {
		fmt.Printf("Error stopping task:\n%v\n", r.Err)
		os.Exit(1)
	}
	fmt.Println("Task removed:")
	fmt.Printf("ID: %s\n", r.ID)
}

func exportTask(ctx *cli.Context) {
	if len(ctx.Args()) != 1 {
		fmt.Print("Incorrect usage\n")
		cli.ShowCommandHelp(ctx, ctx.Command.Name)
		os.Exit(1)
	}
	id := ctx.Args().First()
	task := pClient.GetTask(id)
	if task.Err != nil {
		fmt.Printf("Error exporting task:\n%v\n", task.Err)
		os.Exit(1)
	}
	tb, err := json.Marshal(task)
	if err != nil {
		fmt.Printf("Error exporting task:\n%v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(tb))
}

func enableTask(ctx *cli.Context) {
	if len(ctx.Args()) != 1 {
		fmt.Print("Incorrect usage\n")
		cli.ShowCommandHelp(ctx, ctx.Command.Name)
		os.Exit(1)
	}

	id := ctx.Args().First()
	r := pClient.EnableTask(id)
	if r.Err != nil {
		fmt.Printf("Error enabling task:\n%v\n", r.Err)
		os.Exit(1)
	}
	fmt.Println("Task enabled:")
	fmt.Printf("ID: %s\n", r.ID)
}
