package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gitopsctl",
	Short: "An experimental, lightweight CLI used to reduce toil during GitOps Service development/debugging/support",
	Long: `
'gitopsctl' is an experimental, lightweight CLI for use by the developers of the 
GitOps Service team. (It is not for use by end users/customers, nor would it be
useful to them.)
	
The goal of this tool is to provide reusable commands which can be used to
reduce the toil of supporting/debugging the GitOps Service.
- Downloading the logs from OpenShift CI jobs`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.gitopsctl.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
