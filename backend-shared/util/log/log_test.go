package util

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/log/mocks"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("LogAPIResourceChangeEvent test", func() {
	var log logr.Logger
	var mockCtrl *gomock.Controller
	var mockLog *mocks.MockLogSink
	var logger logr.Logger
	resourceName := "test-name"
	resourceNamespace := "test-namespace"

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockLog = mocks.NewMockLogSink(mockCtrl)
		logger = log.WithSink(mockLog)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("when resource is nil", func() {
		It("should log an error", func() {
			mockLog.EXPECT().WithValues("audit", "true").Return(mockLog)
			mockLog.EXPECT().Error(nil, logError)
			LogAPIResourceChangeEvent(resourceNamespace, resourceName, nil, ResourceCreated, logger)
		})

	})
	Context("when resource is a Secret", func() {
		It("should log the secret resource change", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
			}
			mockLog.EXPECT().WithValues("audit", "true").Return(mockLog)
			mockLog.EXPECT().Enabled(gomock.Any()).Return(true)
			mockLog.EXPECT().Info(0, fmt.Sprintf("API Resource changed for secret resource: %s, name: %s, namespace: %s", ResourceModified, resourceName, resourceNamespace))

			LogAPIResourceChangeEvent(resourceNamespace, resourceName, secret, ResourceModified, logger)
		})
	})

	Context("when the resource can't be marshalled", func() {
		It("should log a marshalling error", func() {
			resource := make(chan int)
			expectedErr := &json.UnsupportedTypeError{
				Type: reflect.TypeOf(resource),
			}

			mockLog.EXPECT().WithValues("audit", "true").Return(mockLog).AnyTimes()
			mockLog.EXPECT().Error(expectedErr, logMarshalJsonError)
			LogAPIResourceChangeEvent(resourceNamespace, resourceName, resource, ResourceCreated, logger)
		})
	})

	Context("when resource is invalid", func() {
		It("should log unmarshaling error", func() {

			resource := []byte(`{"invalid": json}`)

			mockLog.EXPECT().WithValues("audit", "true").Return(mockLog).AnyTimes()
			mockLog.EXPECT().Error(gomock.Any(), logUnMarshalJsonError)

			LogAPIResourceChangeEvent("namespace", "name", resource, ResourceCreated, logger)
		})
	})

	Context("when resource is modified", func() {
		It("should remove managedFields from metadata", func() {

			resource := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceNamespace,
					ManagedFields: []metav1.ManagedFieldsEntry{
						{
							Manager: "test-managedfields",
						},
					},
				},
			}

			expectedJsonRepresentation := []byte(`{"metadata":{"creationTimestamp":null,"managedFields":null,"name":"test-namespace"},"spec":{},"status":{}}`)
			mockLog.EXPECT().WithValues("audit", "true").Return(mockLog).AnyTimes()
			mockLog.EXPECT().Enabled(gomock.Any()).Return(true)
			mockLog.EXPECT().Info(gomock.Any(), fmt.Sprintf("API Resource changed: %s", ResourceModified), "namespace",
				resourceNamespace, "name", resourceName, "object", string(expectedJsonRepresentation))

			LogAPIResourceChangeEvent(resourceNamespace, resourceName, resource, ResourceModified, logger)

		})
	})
})
