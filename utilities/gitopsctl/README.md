
# gitopsctl: a CLI for GitOps Service team members

`gitopsctl` is an experimental, lightweight CLI for use by the developers of the 
GitOps Service team. It is not for use by customers/end users, nor would it be useful to them for any purpose.
	
The goal of this tool is to provide reusable commands which can be used to
reduce the toil of supporting/debugging the GitOps Service.
- Downloading the logs from OpenShift CI jobs
- Parsing JSON-formatted controller logs

Run `gitopsctl --help` for list of commands.


## Development

`gitopsctl` uses `spf13/cobra` for command parsing, and `fatih/color` for ANSI color output.