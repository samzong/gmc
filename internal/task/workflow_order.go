package task

import (
	"sort"
	"strings"
)

func WorkflowNodeOrder(wf WorkflowDefinition) []string {
	seen := map[string]bool{}
	order := make([]string, 0, len(wf.Nodes))
	current := strings.TrimSpace(wf.Start)
	for current != "" && current != "done" && !seen[current] {
		if _, ok := wf.Nodes[current]; !ok {
			break
		}
		seen[current] = true
		order = append(order, current)
		current = strings.TrimSpace(wf.Nodes[current].Next)
	}
	rest := make([]string, 0, len(wf.Nodes))
	for id := range wf.Nodes {
		if !seen[id] {
			rest = append(rest, id)
		}
	}
	sort.Strings(rest)
	return append(order, rest...)
}
