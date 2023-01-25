package gitopserrors

import (
	"context"
	"fmt"
	"runtime/debug"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// UnknownError should be used if there is no useful information we can provide to the user related to the
	// context of the error: for example, if the error is related to specific technical internals of the GitOpsService
	// that we should not expose to the user (for example: 'unable to connect to database')
	UnknownError = "an unknown error occurred"
)

// UserError is an error message that contains both:
// - A GitOps-Service-developer-focused error message
// - A user-focused error message
//
// We differentiate the two, because User Error messages should be sanitized to ensure they do not refer to
// internal technical details, whereas developed-focused-errors do not have this restriction.
//
// Likewise, User Errors tend to contain helpful tips to help the user fix the error. These kind of tips
// are not (as) useful to GitOps Service developers.
//
// A UserError must have both defined. However, if no user error can be provided, use UnknownError as a generic failure message.
type UserError interface {
	DevError() error
	UserError() string
}

type userErrorImpl struct {
	userError string
	devError  error // may be nil
}

// A ConditionError is a UserError that also contains a reason suitable for reporting as a
// https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#Condition.Reason
type ConditionError interface {
	UserError
	ConditionReason() string
}

type conditionErrorImp struct {
	userErrorImpl
	reason string
}

// GitOpsErrorType can be used with Print to output the error message, for debugging purposes
type GitOpsErrorType int

const (
	DevOnly  GitOpsErrorType = iota
	UserOnly GitOpsErrorType = iota
	All      GitOpsErrorType = iota
)

// DevError returns a non-user-facing error, or nil
func (ue userErrorImpl) DevError() error {
	if ue.devError != nil {
		return ue.devError
	}

	return nil
}

// UserError return a user-facing error string, or ""
func (ue userErrorImpl) UserError() string {
	return ue.userError
}

func (ce conditionErrorImp) ConditionReason() string {
	return ce.reason
}

// NewDevOnlyError is used if there is no meaningful information that we can provide the user on why the error occurred
// and/or how to fix it. This function should only be used when the error is primarily referencing internal GitOps Service
// technical details.
func NewDevOnlyError(devError error) UserError {
	return NewUserDevError(UnknownError, devError)
}

// NewUserDevError is used in cases where we can provide both a user-focused error, and a developer-focused error.
// This function should be used when an error occurs, and we can provide both user and dev errors around the context of that error.
func NewUserDevError(userErrorString string, devError error) UserError {

	log := log.FromContext(context.Background())

	if userErrorString == "" {
		diagnosticMsg := fmt.Sprintf("SEVERE: Asked to create an Error with nil user error message. devError message is '%v'.", devError)
		log.Error(nil, diagnosticMsg)
		debug.PrintStack()
	}

	if devError == nil {
		diagnosticMsg := fmt.Sprintf("SEVERE: Asked to create an Error with nil dev error message. userError message is '%v'.", userErrorString)
		log.Error(nil, diagnosticMsg)
		debug.PrintStack()
	}

	return userErrorImpl{
		userError: userErrorString,
		devError:  devError,
	}
}

func NewUserConditionError(userError string, devError error, reason string) ConditionError {
	return conditionErrorImp{
		userErrorImpl: userErrorImpl{
			userError: userError,
			devError:  devError,
		},
		reason: reason,
	}
}

// Print will output the error message, for debugging purposes
func Print(err UserError, filter GitOpsErrorType) {
	switch filter {
	case DevOnly:
		fmt.Println(err.DevError())
	case UserOnly:
		fmt.Println(err.UserError())
	case All:
		fmt.Println(err.UserError())
		fmt.Println(err.DevError())
	}
}
