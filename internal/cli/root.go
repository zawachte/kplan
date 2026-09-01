package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zach/kplan/internal/engine"
	"github.com/zach/kplan/internal/kube"
	"github.com/zach/kplan/internal/manifest"
)

type options struct {
	files        []string
	kubeconfig   string
	context      string
	namespace    string
	fieldManager string
	force        bool
}

func New() *cobra.Command {
	root := &cobra.Command{
		Use:           "kplan",
		Short:         "Plan and apply Kubernetes manifests through the API",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newPlanCommand(), newApplyCommand())
	return root
}

func newPlanCommand() *cobra.Command {
	var opts options
	command := &cobra.Command{
		Use:   "plan -f FILE",
		Short: "Preview server-side apply changes",
		RunE: func(command *cobra.Command, _ []string) error {
			_, changes, err := prepare(command.Context(), opts)
			if err != nil {
				return err
			}
			printPlan(command.OutOrStdout(), changes)
			return nil
		},
	}
	bindFlags(command, &opts)
	return command
}

func newApplyCommand() *cobra.Command {
	var opts options
	var yes bool
	command := &cobra.Command{
		Use:   "apply -f FILE",
		Short: "Plan, confirm, and apply manifests",
		RunE: func(command *cobra.Command, _ []string) error {
			runner, changes, err := prepare(command.Context(), opts)
			if err != nil {
				return err
			}
			printPlan(command.OutOrStdout(), changes)
			objects, err := manifest.Load(opts.files)
			if err != nil {
				return err
			}
			engine.SortForApply(objects)
			if !yes {
				confirmed, err := confirm(command.InOrStdin(), command.OutOrStdout())
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Fprintln(command.OutOrStdout(), "Apply cancelled.")
					return nil
				}
			}
			if err := runner.Apply(command.Context(), objects); err != nil {
				return err
			}
			fmt.Fprintln(command.OutOrStdout(), "Apply complete.")
			return nil
		},
	}
	bindFlags(command, &opts)
	command.Flags().BoolVarP(&yes, "yes", "y", false, "apply without prompting")
	return command
}

func bindFlags(command *cobra.Command, opts *options) {
	command.Flags().StringSliceVarP(&opts.files, "file", "f", nil, "manifest file (repeatable)")
	command.Flags().StringVar(&opts.kubeconfig, "kubeconfig", os.Getenv("KUBECONFIG"), "path to kubeconfig")
	command.Flags().StringVar(&opts.context, "context", "", "kubeconfig context")
	command.Flags().StringVarP(&opts.namespace, "namespace", "n", "", "default namespace")
	command.Flags().StringVar(&opts.fieldManager, "field-manager", "kplan", "server-side apply field manager")
	command.Flags().BoolVar(&opts.force, "force-conflicts", false, "take ownership of conflicting server-side apply fields")
	_ = command.MarkFlagRequired("file")
}

func prepare(ctx context.Context, opts options) (*engine.Engine, []engine.Change, error) {
	objects, err := manifest.Load(opts.files)
	if err != nil {
		return nil, nil, err
	}
	client, err := kube.New(opts.kubeconfig, opts.context, opts.namespace)
	if err != nil {
		return nil, nil, err
	}
	runner := engine.New(client.Dynamic, client.Mapper, client.DefaultNamespace, opts.fieldManager, opts.force)
	changes, err := runner.Plan(ctx, objects)
	if err != nil {
		return nil, nil, err
	}
	return runner, changes, nil
}

func printPlan(output io.Writer, changes []engine.Change) {
	for _, change := range changes {
		name := change.Name
		if change.Namespace != "" {
			name = change.Namespace + "/" + name
		}
		fmt.Fprintf(output, "%-9s %s %s\n", strings.ToUpper(string(change.Action)), change.Kind, name)
		if change.Detail != "" {
			fmt.Fprintf(output, "  reason: %s\n", change.Detail)
		}
		if change.Diff != "" {
			for _, line := range strings.Split(change.Diff, "\n") {
				fmt.Fprintf(output, "  %s\n", line)
			}
		}
	}
	summary := engine.Summary(changes)
	fmt.Fprintf(output, "\nPlan: %d to create, %d to update, %d unchanged, %d conflicts.\n", summary[engine.Create], summary[engine.Update], summary[engine.Unchanged], summary[engine.Conflict])
}

func confirm(input io.Reader, output io.Writer) (bool, error) {
	fmt.Fprint(output, "Apply these changes? [y/N] ")
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes", nil
}
