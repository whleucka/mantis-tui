package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/whleucka/mantis-tui/internal/service"
)

func newFilesCmd(opts *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "files <id>",
		Short: "List an issue's attachments, including those on notes",
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
			files := service.Files(&res.Issue)
			out := cmd.OutOrStdout()
			if opts.json {
				type row struct {
					ID          int    `json:"id"`
					Filename    string `json:"filename"`
					Size        int64  `json:"size"`
					ContentType string `json:"content_type"`
					NoteID      int    `json:"note_id,omitempty"`
				}
				rows := make([]row, len(files))
				for i, f := range files {
					rows[i] = row{f.ID, f.Filename, f.Size, f.ContentType, f.NoteID}
				}
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}
			if len(files) == 0 {
				_, err := fmt.Fprintf(out, "#%d has no attachments\n", id)
				return err
			}
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tSIZE\tATTACHED TO\tNAME")
			for _, f := range files {
				fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", f.ID, service.HumanSize(f.Size), f.Where(), f.Filename)
			}
			return tw.Flush()
		},
	}
}

func newDownloadCmd(opts *globalOpts) *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "download <id> [file-id...]",
		Short: "Download an issue's attachments (all of them without file ids)",
		Long: "Download attachments of an issue, including those on notes, into a directory.\n" +
			"Each is saved as <file-id>-<name> with mode 0600, and its path is printed.",
		Args: minArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs(args)
			if err != nil {
				return err
			}
			id, want := ids[0], ids[1:]
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
			files := service.Files(&res.Issue)
			if len(want) > 0 {
				byID := map[int]service.FileRef{}
				for _, f := range files {
					byID[f.ID] = f
				}
				files = files[:0:0]
				for _, fid := range want {
					f, ok := byID[fid]
					if !ok {
						return usageErrorf("issue #%d has no file %d (see: mantis-tui files %d)", id, fid, id)
					}
					files = append(files, f)
				}
			}
			for _, f := range files {
				path, err := service.Download(ctx, s.client, id, f.Attachment, dir, true)
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), path)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&dir, "output", "o", ".", "directory to save into (created 0700 if missing)")
	return cmd
}
