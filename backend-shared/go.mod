module github.com/redhat-appstudio/managed-gitops/backend-shared

go 1.16

require (
	github.com/emicklei/go-restful/v3 v3.7.1
	github.com/go-pg/pg/extra/pgdebug v0.2.0
	github.com/go-pg/pg/v10 v10.10.6
	github.com/google/uuid v1.3.0
	github.com/stretchr/testify v1.7.0
	golang.org/x/net v0.0.0-20210805182204-aaa1db679c0d // indirect
	golang.org/x/sys v0.0.0-20211007075335-d3039528d8ac // indirect
	k8s.io/apimachinery v0.22.4
	sigs.k8s.io/controller-runtime v0.9.2
)
