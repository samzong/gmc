package cmd

import (
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/samzong/gmc/internal/exitcode"
	"github.com/samzong/gmc/internal/task"
	"github.com/spf13/cobra"
)

var (
	taskAgent      string
	taskModel      string
	taskMode       string
	taskBaseBranch string
	taskAddFile    string
	taskAdvanceTo  string
	taskRmForce    bool
)

var taskCmd = &cobra.Command{
	Use:     "task",
	Short:   "Manage local AI coding tasks",
	GroupID: "worktree",
	Long: `Manage local AI coding tasks backed by a repo-family ledger.

Minimal flow: add -> start -> advance -> attach/show/list -> rm.
Workflow nodes come from ~/.config/gmc/workflow.yaml or ~/.gmc/workflow.yaml.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

var taskAddCmd = &cobra.Command{
	Use:   "add [issue-or-todo-or-text]",
	Short: "Add a task",
	Args:  validateTaskAddArgs,
	Example: `  gmc task add 76
  gmc task add todo.md
  gmc task add --file todo.md
  gmc task add "Fix flaky wt list test"`,
	RunE: runTaskAdd,
}

var taskStartCmd = &cobra.Command{
	Use:   "start <task-id>",
	Short: "Create worktree and start agent",
	Args:  cobra.ExactArgs(1),
	Example: `  gmc task start t-20260614-120000-a1b2
  gmc task start t-20260614-120000-a1b2 --agent codex --model gpt-5
  gmc task start 1 --agent cursor-agent --mode plan`,
	RunE: runTaskStart,
}

var taskListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List tasks",
	Args:    cobra.NoArgs,
	RunE:    runTaskList,
}

var taskShowCmd = &cobra.Command{
	Use:   "show <task-id>",
	Short: "Show task detail",
	Args:  cobra.ExactArgs(1),
	RunE:  runTaskShow,
}

var taskAttachCmd = &cobra.Command{
	Use:   "attach <task-id>",
	Short: "Attach to task agent session",
	Args:  cobra.ExactArgs(1),
	RunE:  runTaskAttach,
}

var taskAdvanceCmd = &cobra.Command{
	Use:   "advance <task-id>",
	Short: "Advance task to the next workflow node",
	Args:  cobra.ExactArgs(1),
	Example: `  gmc task advance t-20260614-120000-a1b2
  gmc task advance 1
  gmc task advance 1 --to review`,
	RunE: runTaskAdvance,
}

var taskRmCmd = &cobra.Command{
	Use:   "rm <task-id>",
	Short: "Delete a task",
	Args:  cobra.ExactArgs(1),
	Example: `  gmc task rm t-20260614-120000-a1b2
  gmc task rm t-20260614-120000-a1b2 --force`,
	RunE: runTaskRm,
}

func init() {
	taskCmd.AddCommand(taskAddCmd)
	taskCmd.AddCommand(taskStartCmd)
	taskCmd.AddCommand(taskListCmd)
	taskCmd.AddCommand(taskShowCmd)
	taskCmd.AddCommand(taskAttachCmd)
	taskCmd.AddCommand(taskAdvanceCmd)
	taskCmd.AddCommand(taskRmCmd)

	taskAddCmd.Flags().StringVar(&taskAddFile, "file", "", "Read task source from file")
	_ = taskAddCmd.MarkFlagFilename("file")

	taskStartCmd.Flags().StringVar(&taskAgent, "agent", "codex", "Agent command: codex, grok, cursor-agent, or opencode")
	taskStartCmd.Flags().StringVar(&taskModel, "model", "", "Model name for the agent")
	taskStartCmd.Flags().StringVar(&taskMode, "mode", "coding", "Agent mode")
	taskStartCmd.Flags().StringVarP(&taskBaseBranch, "base", "b", "", "Base branch for the task worktree")
	_ = taskStartCmd.RegisterFlagCompletionFunc("agent", completeTaskAgents)

	taskAdvanceCmd.Flags().StringVar(&taskAdvanceTo, "to", "", "Workflow node to advance to")

	taskRmCmd.Flags().BoolVarP(&taskRmForce, "force", "f", false, "Force worktree removal if dirty")

	rootCmd.AddCommand(taskCmd)
}

func validateTaskAddArgs(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(taskAddFile) != "" {
		if len(args) > 0 {
			return errors.New("--file cannot be used with a text argument")
		}
		return nil
	}
	return cobra.ExactArgs(1)(cmd, args)
}

func completeTaskAgents(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	_ = cmd
	_ = args
	candidates := []string{"codex", "grok", "cursor-agent", "opencode"}
	return completeStrings(candidates, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeStrings(candidates []string, prefix string) []string {
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

func newTaskEngine() (*task.Engine, error) {
	wt := newWorktreeClient()
	store, err := task.OpenStore(wt)
	if err != nil {
		return nil, exitcode.New(exitcode.NotGitRepo, "not inside a git repository", err)
	}
	return task.NewEngine(store, wt), nil
}

func runTaskAdd(cmd *cobra.Command, args []string) error {
	_ = cmd
	engine, err := newTaskEngine()
	if err != nil {
		return err
	}
	source := strings.TrimSpace(taskAddFile)
	if source == "" {
		source = args[0]
	}
	rec, err := engine.CreateTask(source)
	if err != nil {
		return err
	}
	if outputFormat() == "json" {
		return printJSON(outWriter(), rec)
	}
	fmt.Fprintf(outWriter(), "Added task %s (%s)\n", rec.ID, rec.State)
	fmt.Fprintf(outWriter(), "  title: %s\n", task.DisplayTitle(rec))
	if rec.SourceFile != "" {
		fmt.Fprintf(outWriter(), "  source file: %s\n", rec.SourceFile)
	}
	fmt.Fprintf(outWriter(), "  next: gmc task start %s\n", rec.ID)
	return nil
}

func runTaskStart(cmd *cobra.Command, args []string) error {
	_ = cmd
	engine, err := newTaskEngine()
	if err != nil {
		return err
	}
	sum, err := engine.Start(task.StartOptions{
		TaskID:     args[0],
		Agent:      taskAgent,
		Model:      taskModel,
		Mode:       taskMode,
		BaseBranch: taskBaseBranch,
	})
	if err != nil {
		return err
	}
	if outputFormat() == "json" {
		return printJSON(outWriter(), sum)
	}
	fmt.Fprintf(outWriter(), "Started task %s (%s)\n", sum.Task.ID, sum.Task.State)
	if sum.Task.Workflow != "" {
		fmt.Fprintf(outWriter(), "  workflow: %s\n", sum.Task.Workflow)
	}
	if sum.Task.CurrentNode != "" {
		fmt.Fprintf(outWriter(), "  node: %s\n", sum.Task.CurrentNode)
	}
	if sum.Attempt != nil {
		fmt.Fprintf(outWriter(), "  worktree: %s\n", sum.Attempt.Worktree)
		fmt.Fprintf(outWriter(), "  branch: %s\n", sum.Attempt.Branch)
		fmt.Fprintf(outWriter(), "  task brief: %s\n", sum.Attempt.ContextFile)
		fmt.Fprintf(outWriter(), "  attach: gmc task attach %s\n", sum.Task.ID)
	}
	return nil
}

func runTaskList(cmd *cobra.Command, args []string) error {
	_ = cmd
	_ = args
	engine, err := newTaskEngine()
	if err != nil {
		return err
	}
	summaries, err := engine.ListTasks()
	if err != nil {
		return err
	}
	if outputFormat() == "json" {
		return printJSON(outWriter(), summaries)
	}
	if len(summaries) == 0 {
		fmt.Fprintln(outWriter(), "No tasks found.")
		return nil
	}
	w := tabwriter.NewWriter(outWriter(), 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "#\tID\tSTATE\tAGENT\tMODE\tTITLE")
	for i, sum := range summaries {
		agent, mode := "-", "-"
		if sum.Attempt != nil {
			if sum.Attempt.Agent != "" {
				agent = sum.Attempt.Agent
			}
			if sum.Attempt.Mode != "" {
				mode = sum.Attempt.Mode
			}
		}
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n",
			i+1, sum.Task.ID, sum.Task.State, agent, mode, task.DisplayTitle(sum.Task))
	}
	_ = w.Flush()
	return nil
}

func runTaskShow(cmd *cobra.Command, args []string) error {
	_ = cmd
	engine, err := newTaskEngine()
	if err != nil {
		return err
	}
	sum, err := engine.ShowTask(args[0])
	if err != nil {
		return err
	}
	if outputFormat() == "json" {
		return printJSON(outWriter(), sum)
	}
	printTaskDetail(sum)
	return nil
}

func runTaskAttach(cmd *cobra.Command, args []string) error {
	_ = cmd
	engine, err := newTaskEngine()
	if err != nil {
		return err
	}
	return engine.Attach(args[0])
}

func runTaskAdvance(cmd *cobra.Command, args []string) error {
	_ = cmd
	engine, err := newTaskEngine()
	if err != nil {
		return err
	}
	sum, err := engine.Advance(task.AdvanceOptions{
		TaskID: args[0],
		ToNode: taskAdvanceTo,
	})
	if err != nil {
		return err
	}
	if outputFormat() == "json" {
		return printJSON(outWriter(), sum)
	}
	fmt.Fprintf(outWriter(), "Advanced task %s to %s\n", sum.Task.ID, sum.Task.State)
	if sum.Attempt != nil && sum.Attempt.TmuxSession != "" {
		fmt.Fprintf(outWriter(), "  tmux: %s\n", sum.Attempt.TmuxSession)
		fmt.Fprintf(outWriter(), "  attach: gmc task attach %s\n", sum.Task.ID)
	}
	return nil
}

func runTaskRm(cmd *cobra.Command, args []string) error {
	_ = cmd
	engine, err := newTaskEngine()
	if err != nil {
		return err
	}
	if err := engine.Remove(args[0], task.RemoveOptions{Force: taskRmForce}); err != nil {
		return err
	}
	if outputFormat() == "json" {
		return printJSON(outWriter(), map[string]string{"task_id": args[0], "action": "removed"})
	}
	fmt.Fprintf(outWriter(), "Removed task %s\n", args[0])
	return nil
}

func printTaskDetail(sum task.Summary) {
	fmt.Fprintf(outWriter(), "Task: %s\n", sum.Task.ID)
	fmt.Fprintf(outWriter(), "  title: %s\n", task.DisplayTitle(sum.Task))
	fmt.Fprintf(outWriter(), "  state: %s\n", sum.Task.State)
	if sum.Task.Issue != "" {
		fmt.Fprintf(outWriter(), "  issue: #%s\n", sum.Task.Issue)
	}
	if sum.Task.SourceFile != "" {
		fmt.Fprintf(outWriter(), "  source file: %s\n", sum.Task.SourceFile)
	}
	if sum.Task.Workflow != "" {
		fmt.Fprintf(outWriter(), "  workflow: %s\n", sum.Task.Workflow)
	}
	if sum.Task.CurrentNode != "" {
		fmt.Fprintf(outWriter(), "  node: %s\n", sum.Task.CurrentNode)
	}
	if sum.Attempt != nil {
		fmt.Fprintf(outWriter(), "Attempt: %s\n", sum.Attempt.ID)
		fmt.Fprintf(outWriter(), "  worktree: %s\n", sum.Attempt.Worktree)
		fmt.Fprintf(outWriter(), "  branch: %s\n", sum.Attempt.Branch)
		fmt.Fprintf(outWriter(), "  agent: %s\n", sum.Attempt.Agent)
		if sum.Attempt.Model != "" {
			fmt.Fprintf(outWriter(), "  model: %s\n", sum.Attempt.Model)
		}
		if sum.Attempt.Mode != "" {
			fmt.Fprintf(outWriter(), "  mode: %s\n", sum.Attempt.Mode)
		}
		if sum.Attempt.ContextFile != "" {
			fmt.Fprintf(outWriter(), "  task brief: %s\n", sum.Attempt.ContextFile)
		}
		if sum.Attempt.TmuxSession != "" {
			fmt.Fprintf(outWriter(), "  tmux: %s\n", sum.Attempt.TmuxSession)
		}
		if len(sum.Attempt.TmuxSessions) > 0 {
			fmt.Fprintln(outWriter(), "  tmux sessions:")
			for _, session := range sum.Attempt.TmuxSessions {
				fmt.Fprintf(outWriter(), "    %s\t%s\t%s\n", session.Node, session.Agent, session.Session)
			}
		}
	}
	fmt.Fprintln(outWriter(), "Source:")
	fmt.Fprintln(outWriter(), indentLines(sum.Task.Source, "  "))
}

func indentLines(s, prefix string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		fmt.Fprintf(&b, "%s%s\n", prefix, line)
	}
	return strings.TrimSuffix(b.String(), "\n")
}
