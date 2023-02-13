# init-container

This folder contains an optional Go module that can be run as an init-container for one of the controllers of the GitOps Service.

This can be used to perform maintenance tasks (such as database updates) before the controller is started, by using the initContainer functionality of Kubernetes Deployments.