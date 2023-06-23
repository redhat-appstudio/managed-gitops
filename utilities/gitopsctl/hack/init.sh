#!/bin/bash


# These were the commands that were used to initially init this module, and scaffold the CLI commands

go mod init github.com/redhat-appstudio/managed-gitops/utilities/gitopsctl
cobra-cli init
cobra-cli add download
cobra-cli add job -p 'downloadCmd'

