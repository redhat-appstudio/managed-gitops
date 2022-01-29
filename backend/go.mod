module github.com/redhat-appstudio/managed-gitops/backend

go 1.16

require (
	github.com/emicklei/go-restful/v3 v3.7.1
	github.com/go-logr/logr v0.4.0
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/redhat-appstudio/managed-gitops/backend-shared v0.0.0
	github.com/stretchr/testify v1.7.0
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.21.2
	k8s.io/apimachinery v0.21.2
	k8s.io/client-go v0.21.2
	sigs.k8s.io/controller-runtime v0.9.2

)

replace github.com/redhat-appstudio/managed-gitops/backend-shared => ../backend-shared
