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
	taskCreateFile string
	taskRmForce    bool
)

var taskCmd = &cobra.Command{
	Use:     "task",
	Short:   "Manage local AI coding tasks",
	GroupID: "worktree",
	Long: `Manage local AI coding tasks backed by a repo-family ledger.

Minimal flow: create -> start -> mark -> attach/show/list -> rm.
States: new, plan, code, review, ship.`,
	Args: cobra.NoArgs,
}

var taskCreateCmd = &cobra.Command{
	Use:   "create [issue-or-todo-or-text]",
	Short: "Create a task",
	Args:  validateTaskCreateArgs,
	Example: `  gmc task create 76
  gmc task create todo.md
  gmc task create --file todo.md
  gmc task create "Fix flaky wt list test"`,
	RunE: runTaskCreate,
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

var taskMarkCmd = &cobra.Command{
	Use:               "mark <task-id> <state>",
	Short:             "Set task state",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: completeTaskMarkArgs,
	Example: `  gmc task mark t-20260614-120000-a1b2 code
  gmc task mark t-20260614-120000-a1b2 review
  gmc task mark t-20260614-120000-a1b2 ship`,
	RunE: runTaskMark,
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
	taskCmd.AddCommand(taskCreateCmd)
	taskCmd.AddCommand(taskStartCmd)
	taskCmd.AddCommand(taskListCmd)
	taskCmd.AddCommand(taskShowCmd)
	taskCmd.AddCommand(taskAttachCmd)
	taskCmd.AddCommand(taskMarkCmd)
	taskCmd.AddCommand(taskRmCmd)

	taskCreateCmd.Flags().StringVar(&taskCreateFile, "file", "", "Read task source from file")
	_ = taskCreateCmd.MarkFlagFilename("file")

	taskStartCmd.Flags().StringVar(&taskAgent, "agent", "codex", "Agent command: codex, grok, cursor-agent, or opencode")
	taskStartCmd.Flags().StringVar(&taskModel, "model", "", "Model name for the agent")
	taskStartCmd.Flags().StringVar(&taskMode, "mode", "coding", "Agent mode")
	taskStartCmd.Flags().StringVarP(&taskBaseBranch, "base", "b", "", "Base branch for the task worktree")
	_ = taskStartCmd.RegisterFlagCompletionFunc("agent", completeTaskAgents)

	taskRmCmd.Flags().BoolVarP(&taskRmForce, "force", "f", false, "Force worktree removal if dirty")

	rootCmd.AddCommand(taskCmd)
}

func validateTaskCreateArgs(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(taskCreateFile) != "" {
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

func completeTaskMarkArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	_ = cmd
	if len(args) == 1 {
		return completeStrings(task.StateValues(), toComplete), cobra.ShellCompDirectiveNoFileComp
	}
	return nil, cobra.ShellCompDirectiveDefault
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

func runTaskCreate(cmd *cobra.Command, args []string) error {
	_ = cmd
	engine, err := newTaskEngine()
	if err != nil {
		return err
	}
	source := strings.TrimSpace(taskCreateFile)
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
	fmt.Fprintf(outWriter(), "Created task %s (%s)\n", rec.ID, rec.State)
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

func runTaskMark(cmd *cobra.Command, args []string) error {
	_ = cmd
	engine, err := newTaskEngine()
	if err != nil {
		return err
	}
	sum, err := engine.Mark(args[0], args[1])
	if err != nil {
		return err
	}
	if outputFormat() == "json" {
		return printJSON(outWriter(), sum)
	}
	fmt.Fprintf(outWriter(), "Marked task %s as %s\n", sum.Task.ID, sum.Task.State)
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
