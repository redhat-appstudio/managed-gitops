
# Managed GitOps Test Data

The child folders contain test data used by unit or E2E tests of the GitOps Service. 

As of this writing, they are sample GitOps repositories:
* *sample-gitops-repository*: A sample GitOps respository based on version 1 of the [AppStudio GitOps Repository template](https://github.com/redhat-appstudio/gitops-repository-template). 
    * This version of the template made the assumption that all Components of an Application would be deployed together, as a single Argo CD Application (GitOpsDeployment)
* *component-based-gitops-repository*: A sample GitOps repositories based on version 2 of the [Stonesoup/AppStudio GitOps Repository template](https://github.com/jgwest/gitops-repository-template) 
    * This version of the template makes the assumption that each Component of an Application will be deployed independently, with one Argo CD Application (GitOpsDeployment) per component of an Application.
* *component-based-gitops-repository-no-route*: A copy of *component-based-gitops-repository* with the OpenShift routes removed.  This is required for some E2E tests.  See https://issues.redhat.com/browse/GITOPSRVCE-544 for details.
