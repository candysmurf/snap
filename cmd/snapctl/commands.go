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
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/codegangsta/cli"
)

var (
	commands = []cli.Command{
		{
			Name: "task",
			Subcommands: []cli.Command{
				{
					Name:        "create",
					Description: "Creates a new task in the snap scheduler",
					Usage:       "There are two ways to create a task.\n\t1) Use a task manifest with [--task-manifest]\n\t2) Provide a workflow manifest and schedule details.\n\n\t* Note: Start and stop date/time are optional.\n",
					Action:      createTask,
					Flags: []cli.Flag{
						flTaskManifest,
						flWorkfowManifest,
						flTaskSchedInterval,
						flTaskSchedStartDate,
						flTaskSchedStartTime,
						flTaskSchedStopDate,
						flTaskSchedStopTime,
						flTaskName,
						flTaskSchedDuration,
						flTaskSchedNoStart,
					},
				},
				{
					Name:   "list",
					Usage:  "list",
					Action: listTask,
				},
				{
					Name:   "start",
					Usage:  "start <task_id>",
					Action: startTask,
				},
				{
					Name:   "stop",
					Usage:  "stop <task_id>",
					Action: stopTask,
				},
				{
					Name:   "remove",
					Usage:  "remove <task_id>",
					Action: removeTask,
				},
				{
					Name:   "export",
					Usage:  "export <task_id>",
					Action: exportTask,
				},
				{
					Name:   "watch",
					Usage:  "watch <task_id>",
					Action: watchTask,
				},
				{
					Name:   "enable",
					Usage:  "enable <task_id>",
					Action: enableTask,
				},
			},
		},
		{
			Name: "plugin",
			Subcommands: []cli.Command{
				{
					Name:   "load",
					Usage:  "load <plugin path>",
					Action: loadPlugin,
					Flags: []cli.Flag{
						flPluginAsc,
					},
				},
				{
					Name:   "unload",
					Usage:  "unload",
					Action: unloadPlugin,
					Flags: []cli.Flag{
						flPluginType,
						flPluginName,
						flPluginVersion,
					},
				},
				{
					Name:   "list",
					Usage:  "list",
					Action: listPlugins,
					Flags: []cli.Flag{
						flRunning,
					},
				},
			},
		},
		{
			Name: "metric",
			Subcommands: []cli.Command{
				{
					Name:   "list",
					Usage:  "list",
					Action: listMetrics,
					Flags: []cli.Flag{
						flMetricVersion,
						flMetricNamespace,
					},
				},
				{
					Name:   "get",
					Usage:  "get details on a single metric",
					Action: getMetric,
					Flags: []cli.Flag{
						flMetricVersion,
						flMetricNamespace,
					},
				},
			},
		},
	}

	tribeCommands = []cli.Command{
		{
			Name: "member",
			Subcommands: []cli.Command{
				{
					Name:   "list",
					Usage:  "list",
					Action: listMembers,
				},
				{
					Name:   "show",
					Usage:  "show <member_name>",
					Action: showMember,
					Flags:  []cli.Flag{flVerbose},
				},
			},
		},
		{
			Name: "agreement",
			Subcommands: []cli.Command{
				{
					Name:   "list",
					Usage:  "list",
					Action: listAgreements,
				},
				{
					Name:   "create",
					Usage:  "create <agreement_name>",
					Action: createAgreement,
				},
				{
					Name:   "delete",
					Usage:  "delete <agreement_name>",
					Action: deleteAgreement,
				},
				{
					Name:   "join",
					Usage:  "join <agreement_name> <member_name>",
					Action: joinAgreement,
				},
				{
					Name:   "leave",
					Usage:  "leave <agreement_name> <member_name>",
					Action: leaveAgreement,
				},
				{
					Name:   "members",
					Usage:  "members <agreement_name>",
					Action: agreementMembers,
				},
			},
		},
		{
			Name: "plugin-config",
			Subcommands: []cli.Command{
				{
					Name:   "get",
					Usage:  "get",
					Action: getConfig,
					Flags: []cli.Flag{
						flPluginName,
						flPluginType,
						flPluginVersion,
					},
				},
			},
		},
	}
)

func printFields(tw *tabwriter.Writer, indent bool, width int, fields ...interface{}) {
	var argArray []interface{}
	if indent {
		argArray = append(argArray, strings.Repeat(" ", width))
	}
	for i, field := range fields {
		argArray = append(argArray, field)
		if i < (len(fields) - 1) {
			argArray = append(argArray, "\t")
		}
	}
	fmt.Fprintln(tw, argArray...)
}

// ByCommand contains array of CLI commands.
type ByCommand []cli.Command

func (s ByCommand) Len() int {
	return len(s)
}
func (s ByCommand) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s ByCommand) Less(i, j int) bool {
	if s[i].Name == "help" {
		return false
	}
	if s[j].Name == "help" {
		return true
	}
	return s[i].Name < s[j].Name
}
