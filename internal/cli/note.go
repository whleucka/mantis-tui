package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whleucka/mantis-tui/internal/editor"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/service"
)

func newNoteCmd(opts *globalOpts) *cobra.Command {
	var (
		message string
		edit    bool
		timeLog string
		private bool
		uploads uploadFlags
	)
	cmd := &cobra.Command{
		Use:   "note <id> [-m <text> | --edit | -] [--file PATH]... [--clipboard]",
		Short: "Add a note to an issue (or: note delete <id> <note-id>)",
		Args: func(cmd *cobra.Command, args []string) error {
			return asUsage(cobra.RangeArgs(1, 2)(cmd, args))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			fromStdin := len(args) == 2
			if fromStdin && args[1] != "-" {
				return usageErrorf("unexpected argument %q (use - to read the note from stdin)", args[1])
			}
			withMessage := cmd.Flags().Changed("message")
			sources := 0
			for _, on := range []bool{withMessage, edit, fromStdin} {
				if on {
					sources++
				}
			}
			switch {
			case sources > 1:
				return usageErrorf("use only one of -m, --edit or -")
			case withMessage && strings.TrimSpace(message) == "":
				return usageErrorf("-m must not be empty")
			case sources == 0 && uploads.any():
				// just the attachments: no text, no editor
			case sources == 0 && !opts.deps.isTerminal():
				return usageErrorf("no note text: pass -m <text>, --edit, or - to read stdin")
			case sources == 0:
				edit = true
			}
			if timeLog != "" && !service.ValidDuration(timeLog) {
				return usageErrorf("invalid --time %q (want H:MM, e.g. 0:30)", timeLog)
			}

			s, err := opts.openSession(cmd)
			if err != nil {
				return err
			}
			if timeLog != "" {
				ctx, cancel := opts.ctx(cmd)
				enabled, err := s.meta.TimeTrackingEnabled(ctx)
				cancel()
				if err != nil {
					return err
				}
				if !enabled {
					return usageErrorf("time tracking is disabled on %s; drop --time", s.host.Name)
				}
			}

			ctx, cancel := opts.ctx(cmd)
			files, err := uploads.load(ctx, opts, s)
			cancel()
			if err != nil {
				return err
			}

			text := strings.TrimSpace(message)
			var ed *editor.Session
			switch {
			case fromStdin:
				b, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return fmt.Errorf("read stdin: %w", err)
				}
				if text = strings.TrimSpace(string(b)); text == "" && len(files) == 0 {
					return editor.ErrEmpty
				}
			case edit:
				ed, err = editor.Prepare(editor.Request{
					Host: s.host.Name, IssueID: id, Kind: "note",
					Hints: []string{
						fmt.Sprintf("Note for issue #%d on %s.", id, s.host.Name),
						"Lines starting with '# ' like these are removed. Save an empty file to cancel.",
					},
				})
				if err != nil {
					return err
				}
				if err := ed.Run(); err != nil {
					ed.Cleanup()
					return err
				}
				text, err = ed.Text()
				filesOnly := errors.Is(err, editor.ErrEmpty) && len(files) > 0
				if err != nil && !filesOnly {
					ed.Cleanup()
					return err
				}
			}

			ctx, cancel = opts.ctx(cmd)
			defer cancel()
			note, err := s.client.AddNote(ctx, id, mantis.NewNote{Text: text, Private: private, TimeTracking: timeLog, Files: files})
			if err != nil {
				if ed != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "note not sent; your text is saved in %s\n", ed.Path)
				}
				return err
			}
			if ed != nil {
				ed.Cleanup()
			}

			out := cmd.OutOrStdout()
			if opts.json {
				return json.NewEncoder(out).Encode([]map[string]any{{"id": id, "ok": true, "note_id": note.ID, "files": len(files)}})
			}
			fmt.Fprintf(out, "#%d note %d added%s\n", id, note.ID, withFiles(len(files)))
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVarP(&message, "message", "m", "", "note text")
	f.BoolVar(&edit, "edit", false, "write the note in $VISUAL/$EDITOR")
	f.StringVar(&timeLog, "time", "", "time spent, H:MM")
	f.BoolVar(&private, "private", false, "make the note private")
	uploads.register(f)

	cmd.AddCommand(newNoteDeleteCmd(opts))
	return cmd
}

func newNoteDeleteCmd(opts *globalOpts) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <id> <note-id>",
		Short: "Delete a note",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			noteID, err := parseID(args[1])
			if err != nil {
				return err
			}
			s, err := opts.openSession(cmd)
			if err != nil {
				return err
			}
			if err := opts.confirm(cmd, fmt.Sprintf("Delete note %d from issue #%d?", noteID, id), yes); err != nil {
				return err
			}
			ctx, cancel := opts.ctx(cmd)
			defer cancel()
			if err := s.client.DeleteNote(ctx, id, noteID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "#%d note %d deleted\n", id, noteID)
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "do not ask for confirmation")
	return cmd
}
