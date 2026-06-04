package cmd

import (
	"fmt"
	"strconv"
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
	taskAttempt    string
	taskRefresh    bool
	taskToState    string
	taskGCDryRun   bool
	taskSession    string
	taskRmForce    bool
	taskWatchLines int
)

var taskCmd = &cobra.Command{
	Use:     "task",
	Short:   "Local task control plane for parallel AI coding",
	GroupID: "worktree",
	Long: `Manage tasks, attempts, and runs backed by a repo-family ledger.

State machine: intake → running → reviewing → verifying → ready-for-pr → done.
Use start (from intake), advance (stage tools), watch/attach, rm, gc.`,
	Args: cobra.NoArgs,
}

var taskCreateCmd = &cobra.Command{
	Use:   "create <issue-or-text>",
	Short: "Create a task from an issue number or description",
	Args:  cobra.ExactArgs(1),
	Example: `  gmc task create 76
  gmc task create "Fix flaky wt list test"`,
	RunE: runTaskCreate,
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks in the current repository family",
	Args:  cobra.NoArgs,
	RunE:  runTaskList,
}

var taskShowCmd = &cobra.Command{
	Use:   "show <task-id>",
	Short: "Show one task with attempts and runs",
	Args:  cobra.ExactArgs(1),
	RunE:  runTaskShow,
}

var taskStartCmd = &cobra.Command{
	Use:   "start <task-id>",
	Short: "Create worktree and start an interactive agent session",
	Args:  cobra.ExactArgs(1),
	Example: `  gmc task start fix-flaky-20260101-120000 --agent codex --model gpt-5
  gmc task start fix-flaky-20260101-120000 --agent custom --mode ./scripts/agent.sh`,
	RunE: runTaskStart,
}

var taskAdvanceCmd = &cobra.Command{
	Use:   "advance <task-id>",
	Short: "Advance task to the next stage (runs stage tools when required)",
	Args:  cobra.ExactArgs(1),
	Example: `  gmc task advance t-20260603-fdbf
  gmc task advance t-20260603-fdbf --to verifying`,
	RunE: runTaskAdvance,
}

var taskWatchCmd = &cobra.Command{
	Use:   "watch <task-id>",
	Short: "View attempt tmux output read-only (no needs-human)",
	Args:  cobra.ExactArgs(1),
	Example: `  gmc task watch t-20260603-a1b2
  gmc task watch t-20260603-a1b2 --lines 80`,
	RunE: runTaskWatch,
}

var taskAttachCmd = &cobra.Command{
	Use:   "attach <task-id>",
	Short: "Attach to intervene (marks needs-human)",
	Long: `Interactive attach for when you need to steer the agent.

Marks the task as needs-human. To only view output without changing state, use 'gmc task watch'.`,
	Args: cobra.ExactArgs(1),
	RunE: runTaskAttach,
}

var taskDetachCmd = &cobra.Command{
	Use:   "detach <task-id>",
	Short: "Mark a human attach session as detached in the ledger",
	Args:  cobra.ExactArgs(1),
	RunE:  runTaskDetach,
}

var taskRefreshCmd = &cobra.Command{
	Use:   "refresh [task-id]",
	Short: "Reconcile ledger state with tmux and worktrees",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runTaskRefresh,
}

var taskGCCmd = &cobra.Command{
	Use:   "gc",
	Short: "Garbage-collect runtime resources (dry-run in Phase 1)",
	Args:  cobra.NoArgs,
	RunE:  runTaskGC,
}

var taskRmCmd = &cobra.Command{
	Use:   "rm <task-id>",
	Short: "Archive or delete a task from the ledger",
	Args:  cobra.ExactArgs(1),
	Example: `  gmc task rm t-20260603-a1b2
  gmc task rm t-20260603-a1b2 --force`,
	RunE: runTaskRm,
}

func init() {
	taskCmd.AddCommand(taskCreateCmd)
	taskCmd.AddCommand(taskListCmd)
	taskCmd.AddCommand(taskShowCmd)
	taskCmd.AddCommand(taskStartCmd)
	taskCmd.AddCommand(taskAdvanceCmd)
	taskCmd.AddCommand(taskWatchCmd)
	taskCmd.AddCommand(taskAttachCmd)
	taskCmd.AddCommand(taskDetachCmd)
	taskCmd.AddCommand(taskRefreshCmd)
	taskCmd.AddCommand(taskRmCmd)
	taskCmd.AddCommand(taskGCCmd)

	taskListCmd.Flags().BoolVar(&taskRefresh, "refresh", false, "Reconcile before listing")
	taskShowCmd.Flags().BoolVar(&taskRefresh, "refresh", false, "Reconcile before showing")

	taskStartCmd.Flags().StringVar(&taskAgent, "agent", "codex", "Agent adapter (codex, claude, opencode, custom)")
	taskStartCmd.Flags().StringVar(&taskModel, "model", "", "Model name for the agent")
	taskStartCmd.Flags().StringVar(&taskMode, "mode", "coding", "Agent mode or custom executable when agent=custom")
	taskStartCmd.Flags().StringVarP(&taskBaseBranch, "base", "b", "", "Base branch for the attempt worktree")

	taskAdvanceCmd.Flags().StringVar(&taskToState, "to", "", "Target state (default: next stage)")

	taskWatchCmd.Flags().IntVarP(&taskWatchLines, "lines", "n", 0,
		"Print the last N lines and exit (non-interactive snapshot)")
	taskWatchCmd.Flags().StringVar(&taskAttempt, "attempt", "", "Attempt id (defaults to the only attempt)")
	taskWatchCmd.Flags().StringVar(&taskSession, "session", "coding", "Tmux session: coding or review")

	taskAttachCmd.Flags().StringVar(&taskAttempt, "attempt", "", "Attempt id (defaults to the only attempt)")
	taskAttachCmd.Flags().StringVar(&taskSession, "session", "coding", "Tmux session: coding or review")
	taskDetachCmd.Flags().StringVar(&taskAttempt, "attempt", "", "Attempt id (defaults to the only attempt)")

	taskGCCmd.Flags().BoolVar(&taskGCDryRun, "dry-run", true, "Preview GC actions without deleting resources")

	taskRmCmd.Flags().BoolVar(&taskRmForce, "force", false,
		"Delete ledger data and remove tmux sessions and worktrees (default: archive only)")

	_ = taskStartCmd.RegisterFlagCompletionFunc("agent", completeTaskAgents)

	rootCmd.AddCommand(taskCmd)
}

func completeTaskAgents(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	_ = cmd
	_ = args
	candidates := []string{"codex", "claude", "claude-code", "opencode", "grok", "custom"}
	var out []string
	for _, c := range candidates {
		if strings.HasPrefix(c, toComplete) {
			out = append(out, c)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
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
	rec, err := engine.CreateTask(args[0])
	if err != nil {
		return err
	}
	if outputFormat() == "json" {
		return printJSON(outWriter(), rec)
	}
	fmt.Fprintf(outWriter(), "Created task %s (state: %s)\n", rec.ID, rec.State)
	fmt.Fprintf(outWriter(), "  Next: gmc task start %s\n", rec.ID)
	fmt.Fprintf(outWriter(), "  title: %s\n", task.DisplayTitle(rec))
	if rec.PR != "" {
		fmt.Fprintf(outWriter(), "  pr: #%s\n", rec.PR)
	}
	if rec.Issue != "" {
		fmt.Fprintf(outWriter(), "  issue: #%s\n", rec.Issue)
	}
	fmt.Fprintf(outWriter(), "  source: %s\n", rec.Source)
	return nil
}

func runTaskList(cmd *cobra.Command, args []string) error {
	_ = cmd
	_ = args
	engine, err := newTaskEngine()
	if err != nil {
		return err
	}
	summaries, err := engine.ListTasks(taskRefresh)
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
	_, _ = fmt.Fprintln(w, "ID\tSTATE\tRUNTIME\tAGENT\tMODE\tTITLE")
	for _, sum := range summaries {
		agent, mode := task.ListAgentMode(sum)
		runtime := task.FormatRuntimeLabel(task.PrimaryRuntimeStatus(sum))
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			sum.Task.ID, sum.Task.State, runtime, agent, mode, task.DisplayTitle(sum.Task))
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
	sum, err := engine.ShowTask(args[0], taskRefresh)
	if err != nil {
		return err
	}
	if outputFormat() == "json" {
		return printJSON(outWriter(), sum)
	}
	printTaskDetail(sum)
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
	fmt.Fprintf(outWriter(), "Started task %s\n", sum.Task.ID)
	for _, a := range sum.Attempts {
		fmt.Fprintf(outWriter(), "  attempt: %s (%s)\n", a.ID, a.State)
		fmt.Fprintf(outWriter(), "  worktree: %s\n", a.Worktree)
		if a.ContextFile != "" {
			fmt.Fprintf(outWriter(), "  task brief: %s\n", a.ContextFile)
		}
		if a.TmuxSession != "" {
			fmt.Fprintf(outWriter(), "  tmux (coding): %s\n", a.TmuxSession)
			fmt.Fprintf(outWriter(), "  watch: gmc task watch %s\n", sum.Task.ID)
			fmt.Fprintf(outWriter(), "  advance: gmc task advance %s  # running → reviewing\n", sum.Task.ID)
		}
	}
	return nil
}

func runTaskAdvance(cmd *cobra.Command, args []string) error {
	_ = cmd
	engine, err := newTaskEngine()
	if err != nil {
		return err
	}
	sum, err := engine.Advance(task.AdvanceOptions{TaskID: args[0], ToState: taskToState})
	if err != nil {
		return err
	}
	if outputFormat() == "json" {
		return printJSON(outWriter(), sum)
	}
	fmt.Fprintf(outWriter(), "Task %s is now %s\n", sum.Task.ID, sum.Task.State)
	if sum.LastResult != nil {
		lr := sum.LastResult
		fmt.Fprintf(outWriter(), "  last run: %s %s (exit %d)\n", lr.RunID, lr.State, lr.ExitCode)
		if sum.LastResult.ArtifactFile != "" {
			fmt.Fprintf(outWriter(), "  artifact: %s\n", sum.LastResult.ArtifactFile)
		}
		if sum.LastResult.Summary != "" {
			fmt.Fprintf(outWriter(), "  summary:\n%s\n", indentLines(sum.LastResult.Summary, "    "))
		}
	}
	return nil
}

func indentLines(s, prefix string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		fmt.Fprintf(&b, "%s%s\n", prefix, line)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func runTaskWatch(cmd *cobra.Command, args []string) error {
	_ = cmd
	engine, err := newTaskEngine()
	if err != nil {
		return err
	}
	return engine.Watch(args[0], taskAttempt, taskWatchLines, task.ParseSessionTarget(taskSession))
}

func runTaskAttach(cmd *cobra.Command, args []string) error {
	_ = cmd
	engine, err := newTaskEngine()
	if err != nil {
		return err
	}
	return engine.Attach(args[0], taskAttempt, task.ParseSessionTarget(taskSession))
}

func runTaskDetach(cmd *cobra.Command, args []string) error {
	_ = cmd
	engine, err := newTaskEngine()
	if err != nil {
		return err
	}
	return engine.Detach(args[0], taskAttempt)
}

func runTaskRefresh(cmd *cobra.Command, args []string) error {
	_ = cmd
	engine, err := newTaskEngine()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		summaries, err := engine.ListTasks(false)
		if err != nil {
			return err
		}
		for _, sum := range summaries {
			if err := engine.ReconcileTask(sum.Task.ID); err != nil {
				return err
			}
		}
		if outputFormat() != "json" {
			fmt.Fprintln(outWriter(), "Refreshed all tasks.")
		}
		return nil
	}
	if err := engine.ReconcileTask(args[0]); err != nil {
		return err
	}
	if outputFormat() != "json" {
		fmt.Fprintf(outWriter(), "Refreshed task %s.\n", args[0])
	}
	return nil
}

func runTaskRm(cmd *cobra.Command, args []string) error {
	_ = cmd
	engine, err := newTaskEngine()
	if err != nil {
		return err
	}
	opts := task.RemoveOptions{Archive: !taskRmForce, Force: taskRmForce}
	if err := engine.Remove(args[0], opts); err != nil {
		return err
	}
	if outputFormat() == "json" {
		return printJSON(outWriter(), map[string]string{
			"task_id": args[0],
			"action":  rmActionLabel(opts),
		})
	}
	if opts.Force {
		fmt.Fprintf(outWriter(), "Removed task %s (ledger and runtime cleanup).\n", args[0])
	} else {
		fmt.Fprintf(outWriter(), "Archived task %s (ledger kept; use --force to delete).\n", args[0])
	}
	return nil
}

func rmActionLabel(opts task.RemoveOptions) string {
	if opts.Force {
		return "force"
	}
	return "archive"
}

func runTaskGC(cmd *cobra.Command, args []string) error {
	_ = cmd
	_ = args
	engine, err := newTaskEngine()
	if err != nil {
		return err
	}
	report, err := engine.GC(task.GCOptions{DryRun: taskGCDryRun})
	if err != nil {
		return err
	}
	if outputFormat() == "json" {
		return printJSON(outWriter(), report)
	}
	if len(report.Items) == 0 {
		fmt.Fprintln(outWriter(), "No GC candidates.")
		return nil
	}
	for _, item := range report.Items {
		status := "skip"
		if item.Would {
			status = "would-remove"
		}
		line := fmt.Sprintf("[%s] %s %s", item.Kind, item.TaskID, item.Detail)
		if item.Blocked != "" {
			line += fmt.Sprintf(" (%s)", item.Blocked)
		} else {
			line += fmt.Sprintf(" (%s)", status)
		}
		fmt.Fprintln(outWriter(), line)
	}
	return nil
}

func printTaskDetail(sum task.Summary) {
	fmt.Fprintf(outWriter(), "Task: %s\n", sum.Task.ID)
	fmt.Fprintf(outWriter(), "  title: %s\n", task.DisplayTitle(sum.Task))
	fmt.Fprintf(outWriter(), "  state: %s\n", sum.Task.State)
	fmt.Fprintf(outWriter(), "  source: %s\n", sum.Task.Source)
	if sum.Task.PR != "" {
		fmt.Fprintf(outWriter(), "  pr: #%s\n", sum.Task.PR)
	}
	if sum.Task.Issue != "" {
		fmt.Fprintf(outWriter(), "  issue: #%s\n", sum.Task.Issue)
	}
	inspect := task.SessionInspect{}
	if len(sum.Attempts) > 0 {
		inspect = task.InspectSessionRuntime(sum.Attempts[len(sum.Attempts)-1], sum.Runs)
		fmt.Fprintf(outWriter(), "Runtime: %s\n", inspect.UserMessage)
	}
	for _, a := range sum.Attempts {
		fmt.Fprintf(outWriter(), "Attempt: %s (%s)\n", a.ID, a.State)
		if a.Agent != "" {
			line := fmt.Sprintf("  agent: %s", a.Agent)
			if a.Model != "" {
				line += fmt.Sprintf("  model: %s", a.Model)
			}
			if a.Mode != "" {
				line += fmt.Sprintf("  mode: %s", a.Mode)
			}
			fmt.Fprintln(outWriter(), line)
		}
		if a.Worktree != "" {
			fmt.Fprintf(outWriter(), "  worktree: %s\n", a.Worktree)
		}
		if a.ContextFile != "" {
			fmt.Fprintf(outWriter(), "  task brief: %s\n", a.ContextFile)
		} else if a.Worktree != "" {
			fmt.Fprintf(outWriter(), "  task brief: %s/%s\n", a.Worktree, task.TaskContextRelPath)
		}
		if a.TmuxSession != "" {
			tmuxLine := fmt.Sprintf("  tmux (coding): %s", a.TmuxSession)
			if a.TmuxSocket != "" {
				tmuxLine += fmt.Sprintf(" (socket: %s)", a.TmuxSocket)
			}
			fmt.Fprintln(outWriter(), tmuxLine)
		}
		if a.ReviewTmuxSession != "" {
			fmt.Fprintf(outWriter(), "  tmux (review): %s\n", a.ReviewTmuxSession)
		}
	}
	if sum.LastResult != nil {
		lr := sum.LastResult
		fmt.Fprintf(outWriter(), "Last result: %s %s exit=%d\n", lr.RunID, lr.RunType, lr.ExitCode)
		if sum.LastResult.ArtifactFile != "" {
			fmt.Fprintf(outWriter(), "  artifact: %s\n", sum.LastResult.ArtifactFile)
		}
		if sum.LastResult.Summary != "" {
			fmt.Fprintf(outWriter(), "  summary: %s\n", strings.ReplaceAll(sum.LastResult.Summary, "\n", " "))
		}
	}
	for _, r := range sum.Runs {
		exit := "?"
		if r.ExitCode != nil {
			exit = strconv.Itoa(*r.ExitCode)
		}
		fmt.Fprintf(outWriter(), "Run: %s %s/%s exit=%s\n", r.ID, r.Type, r.State, exit)
	}
}
