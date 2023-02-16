This document describes the steps to install/upgrade your service using `install-upgrade.sh` script.

**Pre-requisites:**

To execute the script, we would need to have some basic requirements:
- The script is being executed on an Openshift cluster
- The image being used is available on quay.io

**Avaialable flag options:**
- `-i` : provide gitops service image (default: `quay.io/${QUAY_USERNAME}/gitops-service:latest`, QUAY_USERNAME = `redhat-appstudio`)
- `-u` : provide QUAY registry username (default: `redhat-appstudio`). This will replace the value of QUAY_USERNAME in `quay.io/${QUAY_USERNAME}/gitops-service:latest`


**Steps to execute the script:**

To execute the script using default image (`quay.io/redhat-appstudio/gitops-service:latest`) with the below command:
```bash
$ sh install-upgrade.sh
```

To execute the script with a specific image:
```bash
$ sh install-upgrade.sh -i <image_url>

# e.g. sh install-upgrade.sh -i quay.io/isequeir/gitops-service:latest
```

To execute the script using image with latest tag but from custom quay registry:
```bash
$ sh install-upgrade.sh -u <QUAY_USERNAME>

e.g. if QUAY_USERNAME is isequeir, this will use the image quay.io/isequeir/gitops-service:latest
```

