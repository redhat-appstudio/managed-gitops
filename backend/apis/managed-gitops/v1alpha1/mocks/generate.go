package mocks

// This is a dummy source file whose job is to contain the directives to (re)produce the generated
// mock fixtures in this package that come from source files outside of this project (otherwise the
// directives should go in the source files themselves).
// Run `make generate` from the project root.
// Dependency: mockgen, qua:
//    GO111MODULE=on go get github.com/golang/mock/mockgen@latest

//go:generate mockgen -destination ./cr-client.go -package mocks sigs.k8s.io/controller-runtime/pkg/client Client
//go:generate mockgen -destination ./status-writer.go -package mocks sigs.k8s.io/controller-runtime/pkg/client StatusWriter
