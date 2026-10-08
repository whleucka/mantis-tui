package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/service"
)

func newShowCmd(opts *globalOpts) *cobra.Command {
	var notes, history bool
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show an issue",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			s, err := opts.openSession(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := opts.ctx(cmd)
			defer cancel()

			res, err := s.client.GetIssue(ctx, id)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if opts.json {
				return writeRaw(out, res.Raw)
			}
			printIssue(out, res.Issue, notes, history)
			return nil
		},
	}
	cmd.Flags().BoolVar(&notes, "notes", false, "include notes")
	cmd.Flags().BoolVar(&history, "history", false, "include history")
	return cmd
}

func printIssue(w io.Writer, is mantis.Issue, notes, history bool) {
	fmt.Fprintf(w, "#%d %s\n\n", is.ID, is.Summary)

	status := is.Status.Label
	if is.Resolution.Name != "" && is.Resolution.Name != "open" {
		status += " (" + is.Resolution.Label + ")"
	}
	field := func(label, value string) {
		if value != "" {
			fmt.Fprintf(w, "%-17s %s\n", label+":", value)
		}
	}
	field("Project", is.Project.Name)
	field("Category", is.Category.Name)
	field("Status", status)
	field("Priority", is.Priority.Label)
	field("Severity", is.Severity.Label)
	field("Reproducibility", is.Reproducibility.Label)
	field("Reporter", is.Reporter.Display())
	field("Handler", handlerName(is))
	field("Created", formatTime(is.CreatedAt))
	field("Updated", formatTime(is.UpdatedAt))
	if len(is.Tags) > 0 {
		names := make([]string, len(is.Tags))
		for i, t := range is.Tags {
			names[i] = t.Name
		}
		field("Tags", strings.Join(names, ", "))
	}
	for _, r := range is.Relationships {
		field("Relationship", fmt.Sprintf("%s #%d %s", r.Type.Label, r.Issue.ID, r.Issue.Summary))
	}
	for _, a := range is.Attachments {
		field("Attachment", fileLabel(a))
	}
	for _, cf := range is.CustomFields {
		field(cf.Field.Name, cf.Value)
	}

	section := func(title, body string) {
		if strings.TrimSpace(body) != "" {
			fmt.Fprintf(w, "\n%s:\n%s\n", title, indent(body))
		}
	}
	section("Description", is.Description)
	section("Steps to reproduce", is.StepsToReproduce)
	section("Additional information", is.AdditionalInformation)

	if notes {
		fmt.Fprintf(w, "\nNotes (%d):\n", len(is.Notes))
		for _, n := range is.Notes {
			var tags []string
			if n.Private() {
				tags = append(tags, "private")
			}
			if n.TimeTracking != nil && n.TimeTracking.Duration != "" && n.TimeTracking.Duration != "00:00" {
				tags = append(tags, n.TimeTracking.Duration)
			}
			meta := ""
			if len(tags) > 0 {
				meta = " (" + strings.Join(tags, ", ") + ")"
			}
			fmt.Fprintf(w, "\n  [%s] %s%s, note %d\n%s\n", formatTime(n.CreatedAt), n.Reporter.Display(), meta, n.ID, indent(indent(n.Text)))
			for _, a := range n.Attachments {
				fmt.Fprintf(w, "    attachment: %s\n", fileLabel(a))
			}
		}
	}
	if history {
		fmt.Fprintf(w, "\nHistory:\n")
		for _, h := range is.History {
			line := h.Message
			if h.Change != "" {
				line += ": " + h.Change
			}
			fmt.Fprintf(w, "  %s  %-14s %s\n", formatTime(h.CreatedAt), h.User.Name, line)
		}
	}
}

// fileLabel names an attachment with the id that download takes.
func fileLabel(a mantis.Attachment) string {
	return fmt.Sprintf("%s (file %d, %s)", a.Filename, a.ID, service.HumanSize(a.Size))
}

func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "  " + l
	}
	return strings.Join(lines, "\n")
}
