package cmd

import (
	"fmt"
	"strings"

	downloadjob "github.com/redhat-appstudio/managed-gitops/utilities/gitopsctl/implementations/download-job"
	"github.com/spf13/cobra"
)

// jobCmd represents the job command
var jobCmd = &cobra.Command{
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("missing URL argument: use --help flag for details")
		} else if len(args) != 1 {
			return fmt.Errorf("too many arguments: use --help flag for details")
		}

		url := args[0]
		if !strings.HasPrefix(url, "https://") {
			return fmt.Errorf("URL must begin with https://: use --help flag for details")
		}

		return nil
	},
	Use:   "job (url to OpenShift CI job)",
	Short: "Download artifacts (logs, k8s resources) from an OpenShift CI job",
	Long: `
Download artifacts (logs, k8s resources) from an OpenShift CI job.

- The URL should correspond to the page OpenShift CI job page: 
    o The 'OpenShift CI job page' will have the OpenShift logo in the 
      top-left-hand corner, and a title of 
      'pull-ci-redhat-appstudio-(repo name) #(job number)'

- Files will be downloaded into the './downloaded' directory

Examples:
- gitopsctl download job "https://prow.ci.openshift.org/view/gs/origin-ci-test/pr-logs/pull/redhat-appstudio_infra-deployments/2368/pull-ci-redhat-appstudio-infra-deployments-main-appstudio-e2e-tests/1700223153835347968"
`,
	Run: func(cmd *cobra.Command, args []string) {

		url := args[0]

		downloadjob.RunDownloadJobCommand(url)

	},
}

func init() {
	downloadCmd.AddCommand(jobCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// jobCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// jobCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
